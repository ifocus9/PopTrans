package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"translate-plugin/internal/backend"
	"translate-plugin/internal/config"
	applog "translate-plugin/internal/logging"
	win "translate-plugin/internal/platform/windows"
)

const (
	mainWindowClass = "TranslateGoMainWindow"
	trayIconID      = 1001
	hotkeyTranslate = 2001
	hotkeyOCR       = 2002

	menuIDStatus    = 3001
	menuIDHotkey    = 3002
	menuIDOCR       = 3003
	menuIDExit      = 3004
	menuIDToggleOCR = 3005
	menuIDSettings  = 3006

	wmTray           = win.WmApp + 1
	wmTranslateDone  = win.WmApp + 2
	wmTranslateError = win.WmApp + 3
	wmBackendStatus  = win.WmApp + 4
	wmTranslateLoad  = win.WmApp + 5
	wmSettingsClosed = win.WmApp + 6
)

var (
	currentApp  *App
	mainWndProc = syscall.NewCallback(mainWindowProc)
)

type App struct {
	baseDir              string
	cfg                  config.Config
	client               *backend.Client
	supervisor           *backend.Supervisor
	hwnd                 win.HWND
	icon                 win.HWND
	overlay              *SelectionOverlay
	settingsMu           sync.Mutex
	settingsWailsRunning bool
	hotkeysSuspended     bool
	wailsMu              sync.Mutex
	wailsProcesses       map[int]*os.Process
	resultWailsProcess   *os.Process
	resultWailsStatePath string

	statusMu sync.Mutex
	status   string

	resultMu sync.Mutex
	source   string
	result   string
	lastErr  string
	busy     bool
}

type wailsResultPayload struct {
	Source  string `json:"source"`
	Result  string `json:"result"`
	Error   string `json:"error"`
	Loading bool   `json:"loading"`
}

func New() (*App, error) {
	baseDir := config.ResolveBaseDir()
	cfg, err := config.Load(baseDir)
	if err != nil {
		return nil, err
	}
	log.Printf(
		"app config loaded: base_dir=%s hotkey=%q hotkey_display=%q ocr_enabled=%t ocr_hotkey=%q ocr_hotkey_display=%q server_port=%d",
		baseDir,
		cfg.Hotkey,
		cfg.HotkeyDisplay,
		cfg.OcrEnabled,
		cfg.OcrHotkey,
		cfg.OcrHotkeyDisplay,
		cfg.ServerPort,
	)

	client := backend.NewClient(config.BackendURL(cfg.ServerPort))

	return &App{
		baseDir:        baseDir,
		cfg:            cfg,
		client:         client,
		supervisor:     backend.NewSupervisor(baseDir, client),
		status:         "正在初始化...",
		wailsProcesses: make(map[int]*os.Process),
	}, nil
}

func (a *App) Run() error {
	currentApp = a
	log.Printf("app run start")
	if a.resolveWailsExe() == "" {
		return errors.New("required Wails UI translate-wails.exe not found")
	}

	if err := win.RegisterClass(mainWindowClass, mainWndProc); err != nil {
		log.Printf("app register class failed: %v", err)
		return err
	}

	hwnd, err := win.CreateWindow(0, mainWindowClass, "选中翻译 Go 前端", 0, 0, 0, 0, 0, 0, 0, 0)
	if err != nil {
		log.Printf("app create main window failed: %v", err)
		return err
	}
	a.hwnd = hwnd
	log.Printf("app main window created: hwnd=%d", a.hwnd)

	a.icon = win.LoadTrayIcon(filepath.Join(a.baseDir, "icon.ico"))
	if err := win.AddTrayIcon(a.hwnd, trayIconID, a.icon, a.trayTip(), wmTray); err != nil {
		log.Printf("app add tray icon failed: %v", err)
		win.DestroyWindow(a.hwnd)
		return err
	}
	log.Printf("app tray icon added")

	if err := a.registerHotkeys(); err != nil {
		log.Printf("app register hotkeys failed: %v", err)
		win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		win.DeleteTrayIcon(a.hwnd, trayIconID)
		win.DestroyWindow(a.hwnd)
		return err
	}
	log.Printf("app hotkeys registered")

	go a.ensureBackendReady()

	_, err = win.MessageLoop()
	log.Printf("app message loop exited: %v", err)
	return err
}

func (a *App) Close() {
	log.Printf("app close start")
	if a.overlay != nil {
		a.overlay.Close()
	}
	a.stopWailsProcesses()
	win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
	win.UnregisterHotKey(a.hwnd, hotkeyOCR)
	win.DeleteTrayIcon(a.hwnd, trayIconID)
	a.supervisor.Stop()
	log.Printf("app close complete")
}

func (a *App) registerHotkeys() error {
	mainHotkey, err := win.ParseHotkey(a.cfg.Hotkey)
	if err != nil {
		return fmt.Errorf("parse main hotkey: %w", err)
	}
	if err := win.RegisterHotKey(a.hwnd, hotkeyTranslate, mainHotkey); err != nil {
		return err
	}
	log.Printf("app main hotkey registered: expr=%q modifiers=%d keycode=%d", a.cfg.Hotkey, mainHotkey.Modifiers, mainHotkey.KeyCode)

	if a.cfg.OcrEnabled {
		return a.registerOCRHotkey()
	}
	log.Printf("app ocr hotkey skipped: disabled in config")

	return nil
}

func (a *App) registerOCRHotkey() error {
	ocrHotkey, err := win.ParseHotkey(a.cfg.OcrHotkey)
	if err != nil {
		return fmt.Errorf("parse ocr hotkey: %w", err)
	}
	if err := win.RegisterHotKey(a.hwnd, hotkeyOCR, ocrHotkey); err != nil {
		return err
	}
	log.Printf("app ocr hotkey registered: expr=%q modifiers=%d keycode=%d", a.cfg.OcrHotkey, ocrHotkey.Modifiers, ocrHotkey.KeyCode)
	return nil
}

func (a *App) suspendHotkeys() {
	if a.hotkeysSuspended {
		return
	}
	win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
	win.UnregisterHotKey(a.hwnd, hotkeyOCR)
	a.hotkeysSuspended = true
	log.Printf("app hotkeys suspended")
}

func (a *App) resumeHotkeys() error {
	if !a.hotkeysSuspended {
		return nil
	}

	win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
	win.UnregisterHotKey(a.hwnd, hotkeyOCR)
	if err := a.registerHotkeys(); err != nil {
		win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		return err
	}
	a.hotkeysSuspended = false
	log.Printf("app hotkeys resumed")
	return nil
}

func (a *App) ensureBackendReady() {
	log.Printf("app ensure backend ready start")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err := a.supervisor.EnsureRunning(ctx, func(status string) {
		if status == "" {
			status = "等待模型加载中..."
		}
		log.Printf("app backend status callback: %q", status)
		a.setStatus(status)
	})
	if err != nil {
		log.Printf("app ensure backend ready failed: %v", err)
		a.setStatus("本地 API 启动失败")
		a.pushError("", err.Error())
		return
	}

	health, err := a.client.Health(context.Background())
	if err != nil {
		log.Printf("app final backend health failed: %v", err)
		a.setStatus("本地 API 已启动")
		return
	}
	log.Printf("app final backend health: translator_ready=%t translator_status=%q ocr_loaded=%t", health.TranslatorReady, health.TranslatorStatus, health.OCRLoaded)
	if health.TranslatorReady {
		a.setStatus("就绪")
	} else if health.TranslatorStatus != "" {
		a.setStatus(health.TranslatorStatus)
	}
}

func (a *App) handleTranslate() {
	a.resultMu.Lock()
	if a.busy {
		a.resultMu.Unlock()
		return
	}
	a.busy = true
	a.resultMu.Unlock()

	log.Printf("app handle translate start")
	a.setStatus("正在捕获选中文本...")

	go func() {
		defer func() {
			a.resultMu.Lock()
			a.busy = false
			a.resultMu.Unlock()
		}()

		text, err := a.captureSelectedText()
		if err != nil {
			log.Printf("app capture selected text failed: %v", err)
			a.setStatus("未捕获到文本")
			a.pushError("", err.Error())
			return
		}

		text = strings.TrimSpace(text)
		if text == "" {
			log.Printf("app capture selected text empty after trim")
			a.setStatus("未捕获到文本")
			a.pushError("", "未检测到选中文本")
			return
		}
		log.Printf("app capture selected text success: chars=%d preview=%q", len(text), previewText(text, 120))

		a.setStatus("正在翻译...")
		a.pushLoading(text)

		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		result, err := a.client.Translate(ctx, text)
		if err != nil {
			log.Printf("app translate failed: %v", err)
			a.setStatus("翻译失败")
			a.pushError(text, err.Error())
			return
		}

		log.Printf("app translate success: result_chars=%d preview=%q", len(result), previewText(result, 120))
		a.setStatus("就绪")
		a.pushResult(text, result)
	}()
}

func (a *App) captureSelectedText() (string, error) {
	log.Printf("capture selected text: native path start")
	text, err := win.CaptureSelectedText()
	if err == nil && strings.TrimSpace(text) != "" {
		log.Printf("capture selected text: native path success chars=%d preview=%q", len(text), previewText(text, 120))
		return text, nil
	}

	log.Printf("capture selected text: native path failed: %v", err)
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("no selected text detected")
}

func (a *App) handleOCRGo() {
	if !a.cfg.OcrEnabled {
		log.Printf("app handle ocr ignored: disabled")
		a.pushError("", "OCR 已在设置中禁用")
		return
	}

	a.resultMu.Lock()
	if a.busy {
		a.resultMu.Unlock()
		return
	}
	a.busy = true
	a.resultMu.Unlock()

	log.Printf("app handle ocr start")
	a.setStatus("OCR 框选区域...")

	a.overlay = NewSelectionOverlay(a.handleOCRSelection, a.handleOCRCancel)
	if err := a.overlay.Show(a.hwnd); err != nil {
		log.Printf("app show ocr overlay failed: %v", err)
		a.finishBusy()
		a.setStatus("OCR 失败")
		a.pushError("", err.Error())
	}
}

func (a *App) handleOCRCancel() {
	a.overlay = nil
	a.finishBusy()
	a.setStatus("就绪")
}

func (a *App) handleOCRSelection(rect win.Rect) {
	a.overlay = nil
	a.setStatus("OCR 识别中...")

	go func() {
		defer a.finishBusy()

		time.Sleep(80 * time.Millisecond)
		pngBytes, err := win.CaptureScreenPNG(rect)
		if err != nil {
			log.Printf("app ocr screen capture failed: %v", err)
			a.setStatus("OCR 失败")
			a.pushError("", err.Error())
			return
		}

		// Open the loading popup only after the screenshot is captured so the
		// popup cannot appear inside the OCR selection, while still providing
		// immediate feedback during recognition and translation.
		a.pushLoading("")

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		a.setStatus("正在识别 OCR 文本...")
		text, err := a.client.OCRImage(ctx, pngBytes)
		if err != nil {
			log.Printf("app ocr request failed: %v", err)
			a.setStatus("OCR 失败")
			a.pushError("", err.Error())
			return
		}

		text = strings.TrimSpace(text)
		if text == "" {
			log.Printf("app ocr returned empty text")
			a.setStatus("OCR 未识别到文本")
			a.pushError("", "OCR 未识别到文本")
			return
		}

		// Update the existing popup as soon as OCR finishes, then keep the
		// translation section in its loading state while translation runs.
		a.pushLoading(text)
		a.setStatus("正在翻译 OCR 文本...")
		translation, err := a.client.Translate(ctx, text)
		if err != nil {
			log.Printf("app ocr text translate request failed: %v", err)
			a.setStatus("OCR 翻译失败")
			a.pushError(text, err.Error())
			return
		}

		translation = strings.TrimSpace(translation)
		if translation == "" {
			log.Printf("app ocr translate returned empty result")
			a.setStatus("OCR 翻译失败")
			a.pushError(text, "OCR 翻译结果为空")
			return
		}

		log.Printf("app ocr translate success: source_chars=%d result_chars=%d source_preview=%q result_preview=%q", len(text), len(translation), previewText(text, 80), previewText(translation, 80))
		a.setStatus("就绪")
		a.pushResult(text, translation)
	}()
}

func (a *App) finishBusy() {
	a.resultMu.Lock()
	a.busy = false
	a.resultMu.Unlock()
}

func (a *App) pushResult(source, result string) {
	a.resultMu.Lock()
	a.source = source
	a.result = result
	a.lastErr = ""
	a.resultMu.Unlock()
	log.Printf("app push result: source_chars=%d result_chars=%d", len(source), len(result))
	win.PostMessage(a.hwnd, wmTranslateDone, 0, 0)
}

func (a *App) pushError(source, errMsg string) {
	a.resultMu.Lock()
	a.source = source
	a.result = ""
	a.lastErr = errMsg
	a.resultMu.Unlock()
	log.Printf("app push error: source_chars=%d error=%q", len(source), errMsg)
	win.PostMessage(a.hwnd, wmTranslateError, 0, 0)
}

func (a *App) pushLoading(source string) {
	a.resultMu.Lock()
	a.source = source
	a.result = ""
	a.lastErr = ""
	a.resultMu.Unlock()
	log.Printf("app push loading: source_chars=%d preview=%q", len(source), previewText(source, 120))
	win.PostMessage(a.hwnd, wmTranslateLoad, 0, 0)
}

func (a *App) setStatus(status string) {
	a.statusMu.Lock()
	if a.status == status {
		a.statusMu.Unlock()
		return
	}
	prev := a.status
	a.status = status
	a.statusMu.Unlock()
	log.Printf("app status update: %q -> %q", prev, status)
	win.PostMessage(a.hwnd, wmBackendStatus, 0, 0)
}

func (a *App) trayTip() string {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	return "选中翻译 Go 前端 - " + a.status
}

func (a *App) showTrayMenu() {
	ocrToggleText := "启用 OCR"
	if a.cfg.OcrEnabled {
		ocrToggleText = "禁用 OCR"
	}

	items := []win.MenuItem{
		{ID: menuIDStatus, Text: "状态: " + a.status, Disabled: true},
		{ID: menuIDHotkey, Text: "翻译快捷键: " + a.cfg.HotkeyDisplay, Disabled: true},
		{ID: menuIDOCR, Text: "OCR 快捷键: " + a.cfg.OcrHotkeyDisplay, Disabled: true},
		{ID: menuIDToggleOCR, Text: ocrToggleText},
		{ID: menuIDSettings, Text: "设置"},
		{Separator: true},
		{ID: menuIDExit, Text: "退出"},
	}
	win.ShowTrayMenu(a.hwnd, items)
}

func (a *App) openSettings() {
	if err := a.showWailsSettings(); err != nil {
		log.Printf("app show wails settings failed: %v", err)
		a.setStatus("设置窗口启动失败")
		return
	}
	log.Printf("app show wails settings success")
}

func (a *App) showWailsSettings() error {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	if a.settingsWailsRunning {
		return nil
	}

	exePath := a.resolveWailsExe()
	if exePath == "" {
		return fmt.Errorf("translate-wails.exe not found")
	}

	cmd := exec.Command(exePath, "--settings")
	cmd.Dir = filepath.Dir(exePath)
	a.suspendHotkeys()
	if err := cmd.Start(); err != nil {
		if resumeErr := a.resumeHotkeys(); resumeErr != nil {
			log.Printf("app resume hotkeys after wails settings start failed: %v", resumeErr)
		}
		return err
	}
	a.settingsWailsRunning = true
	a.addWailsProcess(cmd.Process)

	go func() {
		err := cmd.Wait()
		a.removeWailsProcess(cmd.Process)
		if err != nil {
			log.Printf("wails settings process exited with error: %v", err)
		} else {
			log.Printf("wails settings process exited")
		}
		a.settingsMu.Lock()
		a.settingsWailsRunning = false
		a.settingsMu.Unlock()
		win.PostMessage(a.hwnd, wmSettingsClosed, 0, 0)
	}()
	return nil
}

func (a *App) reloadSettings() {
	next, err := config.Load(a.baseDir)
	if err != nil {
		log.Printf("app reload settings failed: %v", err)
		a.pushError("", err.Error())
		return
	}
	if next == a.cfg {
		return
	}
	loggingChanged := next.LoggingEnabled != a.cfg.LoggingEnabled
	portChanged := next.ServerPort != a.cfg.ServerPort
	if err := a.applySettings(next); err != nil {
		log.Printf("app apply reloaded settings failed: %v", err)
		a.pushError("", err.Error())
		return
	}
	if loggingChanged || portChanged {
		go a.restartBackendForConfigChange(portChanged)
	}
}

func (a *App) applySettings(next config.Config) error {
	prev := a.cfg
	keepSuspended := a.hotkeysSuspended
	win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
	win.UnregisterHotKey(a.hwnd, hotkeyOCR)

	a.cfg = next
	if err := a.registerHotkeys(); err != nil {
		log.Printf("app apply settings failed, restoring previous config: %v", err)
		win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		a.cfg = prev
		_ = a.registerHotkeys()
		if keepSuspended {
			win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
			win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		}
		_ = config.Save(a.baseDir, prev)
		return err
	}

	if err := config.Save(a.baseDir, a.cfg); err != nil {
		log.Printf("app save settings failed, restoring previous config: %v", err)
		win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		a.cfg = prev
		_ = a.registerHotkeys()
		if keepSuspended {
			win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
			win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		}
		return err
	}

	if keepSuspended {
		win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
	}
	_ = applog.Configure(filepath.Join(a.baseDir, "translate.log"), a.cfg.LoggingEnabled)
	a.setStatus("设置已保存")
	return nil
}

func (a *App) restartBackendForConfigChange(portChanged bool) {
	a.setStatus("正在重启本地服务...")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if portChanged {
		a.supervisor.Stop()
		a.client = backend.NewClient(config.BackendURL(a.cfg.ServerPort))
		a.supervisor = backend.NewSupervisor(a.baseDir, a.client)
	}

	restart := a.supervisor.Restart
	if portChanged {
		restart = func(ctx context.Context, onStatus func(string)) error {
			return a.supervisor.EnsureRunning(ctx, onStatus)
		}
	}
	if err := restart(ctx, func(status string) {
		if status != "" {
			a.setStatus(status)
		}
	}); err != nil {
		a.setStatus("本地服务重启失败")
		a.pushError("", err.Error())
		return
	}
	a.setStatus("设置已生效")
}

func (a *App) toggleOCR() {
	next := !a.cfg.OcrEnabled
	log.Printf("app toggle ocr: %t -> %t", a.cfg.OcrEnabled, next)

	if next {
		a.cfg.OcrEnabled = true
		if !a.hotkeysSuspended {
			if err := a.registerOCRHotkey(); err != nil {
				a.cfg.OcrEnabled = false
				log.Printf("app enable ocr failed: %v", err)
				a.pushError("", err.Error())
				return
			}
		}
	} else {
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		a.cfg.OcrEnabled = false
	}

	if err := config.Save(a.baseDir, a.cfg); err != nil {
		log.Printf("app save ocr config failed: %v", err)
		a.pushError("", err.Error())
		return
	}

	if next {
		a.setStatus("OCR 已启用")
	} else {
		a.setStatus("OCR 已禁用")
	}
}

func (a *App) showTranslation() {
	a.resultMu.Lock()
	source := a.source
	result := a.result
	a.resultMu.Unlock()

	if err := a.showWailsResult(source, result, "", false); err != nil {
		log.Printf("show translation wails popup failed: %v", err)
		a.setStatus("翻译窗口启动失败")
		return
	}
	log.Printf("show translation wails popup success")
}

func (a *App) showError() {
	a.resultMu.Lock()
	source := a.source
	errMsg := a.lastErr
	a.resultMu.Unlock()

	if err := a.showWailsResult(source, "", errMsg, false); err != nil {
		log.Printf("show error wails popup failed: %v", err)
		a.setStatus("错误窗口启动失败")
		return
	}
	log.Printf("show error wails popup success")
}

func (a *App) showLoading() {
	a.resultMu.Lock()
	source := a.source
	a.resultMu.Unlock()

	if err := a.showWailsResult(source, "", "", true); err != nil {
		log.Printf("show loading wails popup failed: %v", err)
		a.setStatus("翻译窗口启动失败")
		return
	}
	log.Printf("show loading wails popup success")
}

func (a *App) showWailsResult(source, result, errMsg string, loading bool) error {
	exePath := a.resolveWailsExe()
	if exePath == "" {
		return fmt.Errorf("translate-wails.exe not found")
	}

	payload := wailsResultPayload{
		Source:  source,
		Result:  result,
		Error:   errMsg,
		Loading: loading,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	a.wailsMu.Lock()
	if a.resultWailsProcess != nil && a.resultWailsStatePath != "" {
		path := a.resultWailsStatePath
		a.wailsMu.Unlock()
		if err := os.WriteFile(path, data, 0600); err != nil {
			return fmt.Errorf("update wails result state: %w", err)
		}
		return nil
	}
	a.wailsMu.Unlock()

	file, err := os.CreateTemp("", "translate-result-*.json")
	if err != nil {
		return err
	}
	path := file.Name()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}

	cmd := exec.Command(exePath, "--result", path)
	cmd.Dir = filepath.Dir(exePath)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(path)
		return err
	}
	a.wailsMu.Lock()
	a.wailsProcesses[cmd.Process.Pid] = cmd.Process
	a.resultWailsProcess = cmd.Process
	a.resultWailsStatePath = path
	a.wailsMu.Unlock()
	go func() {
		err := cmd.Wait()
		a.wailsMu.Lock()
		delete(a.wailsProcesses, cmd.Process.Pid)
		if a.resultWailsProcess != nil && a.resultWailsProcess.Pid == cmd.Process.Pid {
			a.resultWailsProcess = nil
			a.resultWailsStatePath = ""
		}
		a.wailsMu.Unlock()
		_ = os.Remove(path)
		if err != nil {
			log.Printf("wails result process exited with error: %v", err)
		}
	}()
	return nil
}

func (a *App) addWailsProcess(process *os.Process) {
	if process == nil {
		return
	}
	a.wailsMu.Lock()
	a.wailsProcesses[process.Pid] = process
	a.wailsMu.Unlock()
}

func (a *App) removeWailsProcess(process *os.Process) {
	if process == nil {
		return
	}
	a.wailsMu.Lock()
	delete(a.wailsProcesses, process.Pid)
	a.wailsMu.Unlock()
}

func (a *App) stopWailsProcesses() {
	a.wailsMu.Lock()
	processes := make([]*os.Process, 0, len(a.wailsProcesses))
	for _, process := range a.wailsProcesses {
		processes = append(processes, process)
	}
	a.wailsProcesses = make(map[int]*os.Process)
	statePath := a.resultWailsStatePath
	a.resultWailsProcess = nil
	a.resultWailsStatePath = ""
	a.wailsMu.Unlock()

	for _, process := range processes {
		if err := process.Kill(); err != nil {
			log.Printf("stop wails process pid=%d: %v", process.Pid, err)
		}
	}
	if statePath != "" {
		_ = os.Remove(statePath)
	}
}

func (a *App) resolveWailsExe() string {
	candidates := []string{
		filepath.Join(a.baseDir, "build", "bin", "translate-wails.exe"),
		filepath.Join(a.baseDir, "translate-wails.exe"),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "translate-wails.exe"))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func mainWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if currentApp == nil {
		return win.DefWindowProc(win.HWND(hwnd), msg, wParam, lParam)
	}

	if currentApp.overlay != nil && currentApp.overlay.IsWindow(win.HWND(hwnd)) {
		if result, handled := currentApp.overlay.HandleMessage(win.HWND(hwnd), msg, wParam, lParam); handled {
			return result
		}
	}

	switch msg {
	case wmTray:
		if uint32(lParam) == win.WmRButtonUp || uint32(lParam) == win.WmLButtonUp {
			log.Printf("main window proc: tray click event lParam=%d", lParam)
			currentApp.showTrayMenu()
		}
		return 0
	case win.WmCommand:
		switch uint16(wParam & 0xFFFF) {
		case menuIDToggleOCR:
			currentApp.toggleOCR()
			return 0
		case menuIDSettings:
			currentApp.openSettings()
			return 0
		case menuIDExit:
			currentApp.Close()
			win.DestroyWindow(win.HWND(hwnd))
			return 0
		}
	case win.WmHotkey:
		log.Printf("main window proc: hotkey event id=%d", wParam)
		if currentApp.hotkeysSuspended {
			log.Printf("main window proc: hotkey ignored while settings are open")
			return 0
		}
		switch int32(wParam) {
		case hotkeyTranslate:
			currentApp.handleTranslate()
			return 0
		case hotkeyOCR:
			currentApp.handleOCRGo()
			return 0
		}
	case wmTranslateDone:
		log.Printf("main window proc: translate done")
		currentApp.showTranslation()
		return 0
	case wmTranslateError:
		log.Printf("main window proc: translate error")
		currentApp.showError()
		return 0
	case wmTranslateLoad:
		log.Printf("main window proc: translate loading")
		currentApp.showLoading()
		return 0
	case wmBackendStatus:
		if err := win.ModifyTrayIcon(currentApp.hwnd, trayIconID, currentApp.icon, currentApp.trayTip(), wmTray); err != nil {
			log.Printf("update tray icon: %v", err)
			return 0
		}
		log.Printf("tray icon status updated")
		return 0
	case wmSettingsClosed:
		log.Printf("main window proc: wails settings closed")
		currentApp.reloadSettings()
		if err := currentApp.resumeHotkeys(); err != nil {
			log.Printf("resume hotkeys after wails settings closed: %v", err)
			currentApp.pushError("", err.Error())
		}
		return 0
	case win.WmClose:
		log.Printf("main window proc: wm_close")
		currentApp.Close()
		win.DestroyWindow(win.HWND(hwnd))
		return 0
	case win.WmDestroy:
		log.Printf("main window proc: wm_destroy")
		win.PostQuitMessage(0)
		return 0
	}

	return win.DefWindowProc(win.HWND(hwnd), msg, wParam, lParam)
}

func previewText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	"translate-plugin/internal/backend"
	"translate-plugin/internal/config"
	applog "translate-plugin/internal/logging"
	win "translate-plugin/internal/platform/windows"
	"translate-plugin/internal/runtimebundle"
	"translate-plugin/internal/uidaemon"
)

const (
	mainWindowClass = "TranslateGoMainWindow"
	trayIconID      = 1001
	hotkeyTranslate = 2001
	hotkeyOCR       = 2002
	hotkeyLive      = 2003

	menuIDStatus    = 3001
	menuIDHotkey    = 3002
	menuIDOCR       = 3003
	menuIDExit      = 3004
	menuIDToggleOCR = 3005
	menuIDSettings  = 3006
	menuIDTranslate = 3007
	menuIDOCRRun    = 3008
	menuIDLive      = 3009

	overlayWindowTitle = "选中翻译浮层"

	wmTray           = win.WmApp + 1
	wmTranslateDone  = win.WmApp + 2
	wmTranslateError = win.WmApp + 3
	wmBackendStatus  = win.WmApp + 4
	wmTranslateLoad  = win.WmApp + 5
	wmSettingsClosed = win.WmApp + 6
	wmHotkeyPause    = win.WmApp + 7
	wmHotkeyResume   = win.WmApp + 8
	wmOverlayClosed  = win.WmApp + 9
	wmLivePause      = win.WmApp + 10
	wmLiveResume     = win.WmApp + 11
	wmLiveStop       = win.WmApp + 12
	wmOverlayRefresh = win.WmApp + 13
	wmLiveRefresh    = win.WmApp + 14
)

var (
	currentApp  *App
	mainWndProc = syscall.NewCallback(mainWindowProc)

	errOCREmptyResult = errors.New("OCR 未识别到文本")
)

type App struct {
	baseDir          string
	cfg              config.Config
	client           *backend.Client
	supervisor       *backend.Supervisor
	hwnd             win.HWND
	icon             win.HWND
	overlay          *SelectionOverlay
	ui               *uidaemon.Manager
	overlayUI        *uidaemon.Manager
	uiPath           string
	enginePath       string
	hotkeysSuspended bool
	// autostart marks a login-triggered launch (--autostart): warm the AI
	// engine in the background but skip the startup UI window so the process
	// only appears in the tray.
	autostart bool

	statusMu sync.Mutex
	status   string

	resultMu  sync.Mutex
	source    string
	result    string
	lastErr   string
	latencyMs int64
	busy      bool

	overlayMu  sync.Mutex
	overlaySeq int

	shotMu    sync.Mutex
	shotFrame *shotFrame

	liveMu        sync.Mutex
	liveCancel    context.CancelFunc
	livePaused    bool
	liveActive    bool
	liveDragging  bool
	liveDragEpoch int
	liveRefresh   chan struct{}
	liveLast      uidaemon.LivePayload
	liveRect      win.Rect
	liveBarSide   string

	backendIdleMu      sync.Mutex
	backendIdleUsers   int
	backendIdleTimer   *time.Timer
	backendIdleTimeout time.Duration

	startupCancelMu sync.Mutex
	startupCancel   context.CancelFunc
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

	// With a bundled release build this extracts translate-ui.exe and
	// ai_engine.exe into <baseDir>/runtime on first launch.
	runtimePaths, bundleErr := runtimebundle.Ensure(baseDir)
	if bundleErr != nil {
		log.Printf("app extract bundled runtime failed: %v", bundleErr)
	}

	client := backend.NewClient(config.BackendURL(cfg.ServerPort))
	supervisor := backend.NewSupervisor(baseDir, client)
	supervisor.SetEnginePath(runtimePaths.EnginePath)
	app := &App{
		baseDir:            baseDir,
		cfg:                cfg,
		client:             client,
		supervisor:         supervisor,
		uiPath:             runtimePaths.UIPath,
		enginePath:         runtimePaths.EnginePath,
		status:             "正在初始化...",
		backendIdleTimeout: cfg.AIIdleTimeout(),
		autostart:          hasArg("--autostart"),
	}
	app.ui = uidaemon.NewManager(baseDir, app.resolveWailsExe, app.handleUIDaemonEvent)

	// A dedicated overlay/live UI process keeps screenshot and live translation
	// independent from the workbench window (opening/using one never closes or
	// replaces the other).
	overlayDir := filepath.Join(baseDir, "overlay-ui")
	if err := os.MkdirAll(overlayDir, 0o755); err != nil {
		log.Printf("app create overlay endpoint dir failed: %v", err)
	}
	app.overlayUI = uidaemon.NewManager(baseDir, app.resolveWailsExe, app.handleUIDaemonEvent)
	app.overlayUI.SetEndpointDir(overlayDir)
	app.overlayUI.SetOverlayMode(true)
	return app, nil
}

func (a *App) Run() error {
	currentApp = a
	log.Printf("app run start")
	if a.resolveWailsExe() == "" {
		return errors.New("required Wails UI translate-ui.exe not found")
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

	// 打包发行版把 icon.ico 放在 baseDir 根；开发布局图标在 assets/。
	iconPath := filepath.Join(a.baseDir, "icon.ico")
	if _, statErr := os.Stat(iconPath); statErr != nil {
		iconPath = filepath.Join(a.baseDir, "assets", "icon.ico")
	}
	a.icon = win.LoadTrayIcon(iconPath)
	if err := win.AddTrayIcon(a.hwnd, trayIconID, a.icon, a.trayTip(), wmTray); err != nil {
		log.Printf("app add tray icon failed: %v", err)
		win.DestroyWindow(a.hwnd)
		return err
	}
	log.Printf("app tray icon added")

	// 按配置校准开机自启动注册表项：开启时保证 Run 键指向当前 exe（安装
	// 目录移动也能自动修正路径），关闭时清除残留项。
	if err := a.syncLaunchAtLogin(); err != nil {
		log.Printf("app sync launch at login failed: %v", err)
	}

	if err := a.registerHotkeys(); err != nil {
		// A conflicting global hotkey must not prevent the workbench from
		// starting; the user can change it in settings.
		log.Printf("app register hotkeys failed (continuing without them): %v", err)
		win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		win.UnregisterHotKey(a.hwnd, hotkeyLive)
		a.setStatus("快捷键冲突，请在设置中修改")
	} else {
		log.Printf("app hotkeys registered")
	}

	// Start the Wails UI immediately so the user sees model download/load
	// progress during application startup. The UI daemon owns only the loading
	// window here; it hides itself once the backend reports translator_ready.
	// Login autostart skips the UI entirely: the daemon is started on demand
	// by the first tray/hotkey action, so boot only fills the tray.
	// A normal launch opens the workbench immediately (which also warms the
	// engine in the background). Login autostart stays tray-only so it never
	// steals focus at logon.
	if !a.autostart {
		go a.openTranslate()
	}
	go a.ensureBackendReady()
	go a.prewarmOverlayUI()

	_, err = win.MessageLoop()
	log.Printf("app message loop exited: %v", err)
	return err
}

func (a *App) Close() {
	log.Printf("app close start")
	a.startupCancelMu.Lock()
	if a.startupCancel != nil {
		a.startupCancel()
	}
	a.startupCancelMu.Unlock()

	if a.overlay != nil {
		a.overlay.Close()
	}
	a.invalidateOverlay()
	a.stopLiveSession(false)
	if a.overlayUI != nil {
		a.overlayUI.Stop()
	}
	if a.ui != nil {
		a.ui.Stop()
	}
	win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
	win.UnregisterHotKey(a.hwnd, hotkeyOCR)
	win.UnregisterHotKey(a.hwnd, hotkeyLive)
	win.DeleteTrayIcon(a.hwnd, trayIconID)
	a.cancelBackendIdleStop()
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
		if err := a.registerOCRHotkey(); err != nil {
			return err
		}
	} else {
		log.Printf("app ocr hotkey skipped: disabled in config")
	}

	if a.cfg.LiveEnabled {
		if err := a.registerLiveHotkey(); err != nil {
			return err
		}
	} else {
		log.Printf("app live hotkey skipped: disabled in config")
	}

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

func (a *App) registerLiveHotkey() error {
	liveHotkey, err := win.ParseHotkey(a.cfg.LiveHotkey)
	if err != nil {
		return fmt.Errorf("parse live hotkey: %w", err)
	}
	if err := win.RegisterHotKey(a.hwnd, hotkeyLive, liveHotkey); err != nil {
		return err
	}
	log.Printf("app live hotkey registered: expr=%q modifiers=%d keycode=%d", a.cfg.LiveHotkey, liveHotkey.Modifiers, liveHotkey.KeyCode)
	return nil
}

func (a *App) suspendHotkeys() {
	if a.hotkeysSuspended {
		return
	}
	win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
	win.UnregisterHotKey(a.hwnd, hotkeyOCR)
	win.UnregisterHotKey(a.hwnd, hotkeyLive)
	a.hotkeysSuspended = true
	log.Printf("app hotkeys suspended")
}

func (a *App) resumeHotkeys() error {
	if !a.hotkeysSuspended {
		return nil
	}

	win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
	win.UnregisterHotKey(a.hwnd, hotkeyOCR)
	win.UnregisterHotKey(a.hwnd, hotkeyLive)
	if err := a.registerHotkeys(); err != nil {
		win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		win.UnregisterHotKey(a.hwnd, hotkeyLive)
		a.hotkeysSuspended = false
		log.Printf("app hotkeys resume failed (hotkeys unavailable): %v", err)
		a.setStatus("快捷键被占用，暂时无法注册")
		return nil
	}
	a.hotkeysSuspended = false
	log.Printf("app hotkeys resumed")
	return nil
}

func (a *App) ensureBackendReady() {
	log.Printf("app ensure backend ready start")
	ctx, cancel := context.WithTimeout(context.Background(), backend.StartupReadyTimeout)
	defer cancel()

	a.startupCancelMu.Lock()
	a.startupCancel = cancel
	a.startupCancelMu.Unlock()
	defer func() {
		a.startupCancelMu.Lock()
		a.startupCancel = nil
		a.startupCancelMu.Unlock()
	}()

	var health backend.Health
	var healthErr error
	err := a.withBackend(ctx, func(status string) {
		if status == "" {
			status = "等待模型加载中..."
		}
		log.Printf("app backend status callback: %q", status)
		a.setStatus(status)
	}, func(context.Context) error {
		health, healthErr = a.client.Health(context.Background())
		return nil
	})
	if err != nil {
		// Keep the workbench usable: surface the failure in the tray/status and
		// let the next explicit translation action report the error. A blocking
		// startup popup here would leave users with no usable window.
		log.Printf("app ensure backend ready failed: %v", err)
		a.setStatus("本地 API 启动失败")
		return
	}

	if healthErr != nil {
		log.Printf("app final backend health failed: %v", healthErr)
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

	// A text-selection translation takes over the floating window.
	a.stopLiveSession(false)

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

		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		var result string
		var latencyMs int64
		err = a.withBackend(ctx, func(status string) {
			if status == "" {
				status = "等待模型加载中..."
			}
			a.setStatus(status)
		}, func(reqCtx context.Context) error {
			startedAt := time.Now()
			var translateErr error
			result, translateErr = a.client.TranslateWithLanguages(
				reqCtx, text, a.cfg.SourceLanguage, a.cfg.TargetLanguage,
			)
			latencyMs = time.Since(startedAt).Milliseconds()
			return translateErr
		})
		if err != nil {
			log.Printf("app translate failed: %v", err)
			a.setStatus("翻译失败")
			a.pushError(text, err.Error())
			return
		}

		log.Printf("app translate success: result_chars=%d preview=%q", len(result), previewText(result, 120))
		a.setStatus("就绪")
		a.pushResult(text, result, latencyMs)
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

const (
	ocrSelectionHint  = "拖拽框选要翻译的区域 · Esc 取消"
	liveSelectionHint = "拖拽框选实时翻译区域 · Esc 取消"
)

func (a *App) handleOCRGo() {
	if !a.cfg.OcrEnabled {
		log.Printf("app handle ocr ignored: disabled")
		a.pushError("", "OCR 已在设置中禁用")
		return
	}
	if a.overlay != nil {
		return
	}

	a.resultMu.Lock()
	if a.busy {
		a.resultMu.Unlock()
		return
	}
	a.busy = true
	a.resultMu.Unlock()

	// A fresh screenshot supersedes a running live session.
	a.stopLiveSession(true)

	log.Printf("app handle ocr start")
	a.setStatus("OCR 框选区域...")
	// Warm the engine while the user is still selecting, so the first
	// screenshot does not sit behind a cold model start.
	go a.ensureBackendReady()

	a.overlay = NewSelectionOverlay(ocrSelectionHint, a.handleOCRSelection, a.handleOCRCancel)
	if err := a.overlay.Show(a.hwnd); err != nil {
		log.Printf("app show ocr overlay failed: %v", err)
		a.overlay = nil
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
	seq := a.beginOverlay()

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

		// The overlay window matches the selection exactly and shows this frame
		// as its base image, so translated chips can be hidden to reveal the
		// untouched original.
		windowRect, barSide := computeFrameLayout(rect)
		imageDataURL := ""
		if jpegBytes, jpegErr := win.EncodePNGToJPEG(pngBytes, 82); jpegErr == nil {
			imageDataURL = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpegBytes)
		}
		basePayload := uidaemon.OverlayPayload{
			Rect:         screenRect(rect),
			WindowRect:   windowRect,
			BarSide:      barSide,
			ImageWidth:   int(rect.Width()),
			ImageHeight:  int(rect.Height()),
			ImageDataURL: imageDataURL,
			Style:        a.overlayStyle(),
			Loading:      true,
			Stage:        "ocr",
			Progress:     15,
		}
		a.storeShotFrame(basePayload, pngBytes)
		showCtx, showCancel := context.WithTimeout(context.Background(), 20*time.Second)
		if err := a.overlayUI.ShowOverlay(showCtx, basePayload); err != nil {
			log.Printf("app show screenshot overlay failed: %v", err)
		}
		showCancel()
		if !a.overlayCurrent(seq) {
			return
		}

		a.setStatus("正在识别并翻译...")
		reqCtx, reqCancel := context.WithTimeout(context.Background(), 150*time.Second)
		defer reqCancel()

		startedAt := time.Now()
		var result backend.OCRTranslateBlocksResult
		lines := 0
		// Progressive updates rebuild every chip; cache the blurred crops so
		// earlier batches are not blurred and JPEG-encoded again on each pass.
		backdrops := make(map[int]blockBackdrop)
		err = a.withBackend(reqCtx, func(status string) {
			if status == "" {
				status = "等待模型加载中..."
			}
			a.setStatus(status)
		}, func(callCtx context.Context) error {
			ocr, ocrErr := a.client.OCRBlocksWithLanguages(callCtx, pngBytes, a.cfg.SourceLanguage)
			if ocrErr != nil {
				if !errors.Is(ocrErr, backend.ErrOCRBlocksUnsupported) {
					return ocrErr
				}
				// Legacy engine: one combined call, no intermediate stage.
				log.Printf("screenshot ocr endpoint unsupported, using legacy combined path")
				var callErr error
				result, callErr = a.ocrTranslateFrame(callCtx, pngBytes)
				lines = len(result.Blocks)
				return callErr
			}

			lines = len(ocr.LineBoxes)
			a.updateOverlayProgress(seq, basePayload, "translate", 55, lines)
			var callErr error
			result, callErr = a.translateOCRProgressive(callCtx, ocr, false,
				func(partial backend.OCRTranslateBlocksResult, done, total int) {
					if !a.overlayCurrent(seq) {
						return
					}
					payload := a.overlayPayloadFromResult(basePayload, pngBytes, partial, nil, backdrops)
					payload.Loading = true
					payload.Stage = "translate"
					payload.Progress = 55
					if total > 0 {
						payload.Progress = 55 + int(40*float64(done)/float64(total))
						if payload.Progress > 95 {
							payload.Progress = 95
						}
					}
					payload.Lines = total
					progressCtx, progressCancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer progressCancel()
					if err := a.overlayUI.ShowOverlay(progressCtx, payload); err != nil {
						log.Printf("app update partial screenshot overlay failed: %v", err)
					}
				})
			return callErr
		})

		if !a.overlayCurrent(seq) {
			log.Printf("app ocr result dropped: overlay closed")
			return
		}

		update := a.overlayResultPayload(basePayload, pngBytes, result, err, time.Since(startedAt), backdrops)
		switch {
		case err != nil:
			log.Printf("app ocr blocks request failed: %v", err)
			a.setStatus("OCR 失败")
		case update.Source == "":
			log.Printf("app ocr returned empty text")
			a.setStatus("OCR 未识别到文本")
		default:
			log.Printf("app ocr blocks success: blocks=%d source_chars=%d result_chars=%d", len(update.Blocks), len(update.Source), len(update.Translation))
			a.setStatus("就绪")
		}

		updateCtx, updateCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer updateCancel()
		if err := a.overlayUI.ShowOverlay(updateCtx, update); err != nil {
			log.Printf("app update screenshot overlay failed: %v", err)
		}
	}()
}

// blockBackdrop is a blurred crop and its auto-selected text tone. Progressive
// translation reuses cached backdrops for blocks from earlier batches.
type blockBackdrop struct {
	image string
	color string
}

// overlayPayloadFromResult builds the image + chips payload without deciding
// its final progress state. backdrops may be nil; when non-nil it is both read
// and updated so repeated partial payloads only blur newly translated lines.
func (a *App) overlayPayloadFromResult(
	base uidaemon.OverlayPayload,
	pngBytes []byte,
	result backend.OCRTranslateBlocksResult,
	err error,
	backdrops map[int]blockBackdrop,
) uidaemon.OverlayPayload {
	update := uidaemon.OverlayPayload{
		Rect:         base.Rect,
		WindowRect:   base.WindowRect,
		BarSide:      base.BarSide,
		ImageWidth:   base.ImageWidth,
		ImageHeight:  base.ImageHeight,
		ImageDataURL: base.ImageDataURL,
		Source:       strings.TrimSpace(result.SourceText),
		Translation:  strings.TrimSpace(result.Translation),
		Style:        base.Style,
	}
	for index, block := range result.Blocks {
		translated := toTranslatedBlock(block)
		if backdrops != nil && strings.TrimSpace(translated.Translation) != "" {
			if cached, ok := backdrops[index]; ok {
				translated.BackgroundImage = cached.image
				translated.TextColor = cached.color
			}
		}
		update.Blocks = append(update.Blocks, translated)
	}
	clipBlockOverlaps(update.Blocks)
	attachBlurredBackgrounds(pngBytes, update.Blocks)
	if backdrops != nil {
		for index, block := range update.Blocks {
			if block.BackgroundImage != "" {
				backdrops[index] = blockBackdrop{
					image: block.BackgroundImage,
					color: block.TextColor,
				}
			}
		}
	}
	if err != nil {
		update.Error = err.Error()
	}
	return update
}

// overlayResultPayload turns an OCR/translation result into the final overlay
// update, including clipped geometry, blurred backgrounds and progress state.
func (a *App) overlayResultPayload(
	base uidaemon.OverlayPayload,
	pngBytes []byte,
	result backend.OCRTranslateBlocksResult,
	err error,
	latency time.Duration,
	backdrops map[int]blockBackdrop,
) uidaemon.OverlayPayload {
	update := a.overlayPayloadFromResult(base, pngBytes, result, err, backdrops)
	update.LatencyMs = latency.Milliseconds()

	switch {
	case err != nil:
		update.Stage = "error"
		update.Progress = 0
	case update.Source == "":
		update.Error = errOCREmptyResult.Error()
		update.Stage = "error"
		update.Progress = 0
	default:
		update.Stage = "done"
		update.Progress = 100
		update.Lines = len(update.Blocks)
	}
	return update
}

// ocrTranslateFrame prefers the line-level blocks endpoint. Older packaged
// engines only expose /api/v1/ocr + /v1/chat/completions; fall back to the
// legacy two-step path so the overlay still works (as a full-text panel).
func (a *App) ocrTranslateFrame(ctx context.Context, pngBytes []byte) (backend.OCRTranslateBlocksResult, error) {
	result, err := a.client.OCRTranslateBlocksWithLanguages(
		ctx, pngBytes, a.cfg.SourceLanguage, a.cfg.TargetLanguage,
	)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, backend.ErrOCRBlocksUnsupported) {
		return result, err
	}

	log.Printf("blocks endpoint unavailable, falling back to legacy ocr+translate")
	text, ocrErr := a.client.OCRImage(ctx, pngBytes)
	if ocrErr != nil {
		return backend.OCRTranslateBlocksResult{}, ocrErr
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return backend.OCRTranslateBlocksResult{}, errOCREmptyResult
	}
	translation, translateErr := a.client.TranslateWithLanguages(
		ctx, text, a.cfg.SourceLanguage, a.cfg.TargetLanguage,
	)
	if translateErr != nil {
		return backend.OCRTranslateBlocksResult{}, translateErr
	}
	return backend.OCRTranslateBlocksResult{
		SourceText:  text,
		Translation: strings.TrimSpace(translation),
	}, nil
}

// updateOverlayProgress refreshes the screenshot overlay's stage/progress
// without touching its content. It is a no-op once the overlay was closed.
func (a *App) updateOverlayProgress(seq int, base uidaemon.OverlayPayload, stage string, progress, lines int) {
	if !a.overlayCurrent(seq) {
		return
	}
	base.Loading = true
	base.Stage = stage
	base.Progress = progress
	base.Lines = lines
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.overlayUI.ShowOverlay(ctx, base); err != nil {
		log.Printf("app update overlay progress failed: %v", err)
	}
}

func glossaryHits(hits []backend.GlossaryHit) []uidaemon.GlossaryHit {
	if len(hits) == 0 {
		return nil
	}
	result := make([]uidaemon.GlossaryHit, 0, len(hits))
	for _, hit := range hits {
		result = append(result, uidaemon.GlossaryHit{
			Term:        hit.Term,
			Translation: hit.Translation,
			Source:      hit.Source,
			Note:        hit.Note,
		})
	}
	return result
}

func toTranslatedBlock(block backend.OCRBlock) uidaemon.TranslatedBlock {
	sourceHeight := block.LineHeight
	if sourceHeight <= 0 {
		sourceHeight = block.H
	}
	return uidaemon.TranslatedBlock{
		Text:         block.Text,
		Translation:  block.Translation,
		X:            block.X,
		Y:            block.Y,
		W:            block.W,
		H:            block.H,
		Score:        block.Score,
		GlossaryHits: glossaryHits(block.GlossaryHits),
		Angle:        block.Angle,
		CX:           block.CX,
		CY:           block.CY,
		LineWidth:    block.LineWidth,
		LineHeight:   block.LineHeight,
		SourceHeight: sourceHeight,
	}
}

func screenRect(rect win.Rect) uidaemon.ScreenRect {
	return uidaemon.ScreenRect{
		X:      int(rect.Left),
		Y:      int(rect.Top),
		Width:  int(rect.Width()),
		Height: int(rect.Height()),
	}
}

func (a *App) beginOverlay() int {
	a.overlayMu.Lock()
	defer a.overlayMu.Unlock()
	a.overlaySeq++
	return a.overlaySeq
}

func (a *App) overlayCurrent(seq int) bool {
	a.overlayMu.Lock()
	defer a.overlayMu.Unlock()
	return a.overlaySeq == seq
}

func (a *App) invalidateOverlay() {
	a.beginOverlay()
	a.shotMu.Lock()
	a.shotFrame = nil
	a.shotMu.Unlock()
}

// shotFrame keeps the last screenshot so the refresh button can re-run OCR and
// translation without recapturing the screen.
type shotFrame struct {
	base uidaemon.OverlayPayload
	png  []byte
}

func (a *App) storeShotFrame(base uidaemon.OverlayPayload, pngBytes []byte) {
	a.shotMu.Lock()
	a.shotFrame = &shotFrame{base: base, png: pngBytes}
	a.shotMu.Unlock()
}

func (a *App) currentShotFrame() *shotFrame {
	a.shotMu.Lock()
	defer a.shotMu.Unlock()
	return a.shotFrame
}

// refreshScreenshot re-runs OCR and translation for the displayed screenshot.
// force=true bypasses the backend line cache so a manual refresh produces a
// fresh model result.
func (a *App) refreshScreenshot() {
	a.resultMu.Lock()
	if a.busy {
		a.resultMu.Unlock()
		return
	}
	a.shotMu.Lock()
	frame := a.shotFrame
	a.shotMu.Unlock()
	if frame == nil || len(frame.png) == 0 {
		a.resultMu.Unlock()
		return
	}
	a.busy = true
	a.resultMu.Unlock()

	seq := a.beginOverlay()
	base := frame.base
	base.Loading = true
	base.Stage = "ocr"
	base.Progress = 15
	base.Lines = 0
	base.Blocks = nil
	base.Source = ""
	base.Translation = ""
	base.Error = ""
	base.LatencyMs = 0
	a.updateOverlayProgress(seq, base, "ocr", 15, 0)
	a.setStatus("正在刷新译文...")

	go func() {
		defer a.finishBusy()
		reqCtx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
		defer cancel()

		startedAt := time.Now()
		var result backend.OCRTranslateBlocksResult
		callErr := a.withBackend(reqCtx, func(status string) {
			if status != "" {
				a.setStatus(status)
			}
		}, func(callCtx context.Context) error {
			ocr, ocrErr := a.client.OCRBlocksWithLanguages(callCtx, frame.png, a.cfg.SourceLanguage)
			switch {
			case errors.Is(ocrErr, backend.ErrOCRBlocksUnsupported):
				log.Printf("app refresh using legacy combined path")
				var err error
				result, err = a.ocrTranslateFrame(callCtx, frame.png)
				return err
			case ocrErr != nil:
				return ocrErr
			default:
				a.updateOverlayProgress(seq, base, "translate", 55, len(ocr.LineBoxes))
				var err error
				result, err = a.translateOCRResult(callCtx, ocr, true)
				return err
			}
		})

		if !a.overlayCurrent(seq) {
			log.Printf("app refresh result dropped: overlay closed")
			return
		}

		update := a.overlayResultPayload(base, frame.png, result, callErr, time.Since(startedAt), nil)
		switch {
		case callErr != nil:
			log.Printf("app refresh screenshot failed: %v", callErr)
			a.setStatus("刷新失败")
		case update.Source == "":
			log.Printf("app refresh returned empty text")
			a.setStatus("OCR 未识别到文本")
		default:
			log.Printf("app refresh screenshot success: blocks=%d", len(update.Blocks))
			a.setStatus("就绪")
		}

		updateCtx, updateCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer updateCancel()
		if err := a.overlayUI.ShowOverlay(updateCtx, update); err != nil {
			log.Printf("app update refreshed overlay failed: %v", err)
		}
	}()
}

// ── 区域实时翻译 ──────────────────────────────────────────

func (a *App) handleLiveHotkey() {
	a.liveMu.Lock()
	active := a.liveActive
	a.liveMu.Unlock()

	if active {
		a.stopLiveSession(true)
		return
	}
	if !a.cfg.LiveEnabled {
		a.pushError("", "实时翻译已在设置中禁用")
		return
	}
	if a.overlay != nil {
		return
	}
	a.resultMu.Lock()
	busy := a.busy
	a.resultMu.Unlock()
	if busy {
		log.Printf("app live translation ignored: another ocr/translate is running")
		return
	}

	// Drop any stale one-shot screenshot result before taking over.
	a.invalidateOverlay()

	log.Printf("app live translation selection start")
	a.setStatus("实时翻译框选区域...")
	go a.ensureBackendReady()

	a.overlay = NewSelectionOverlay(liveSelectionHint, a.startLiveSession, func() {
		a.overlay = nil
		a.setStatus("就绪")
	})
	if err := a.overlay.Show(a.hwnd); err != nil {
		log.Printf("app show live selection overlay failed: %v", err)
		a.overlay = nil
		a.setStatus("实时翻译失败")
		a.pushError("", err.Error())
	}
}

func (a *App) overlayStyle() uidaemon.OverlayStyle {
	return uidaemon.OverlayStyle{
		FontFamily: a.cfg.OverlayFontFamily,
		FontWeight: a.cfg.OverlayFontWeight,
	}
}

func (a *App) prewarmOverlayUI() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.overlayUI.EnsureReady(ctx); err != nil {
		log.Printf("app prewarm overlay ui failed: %v", err)
	}
}

func (a *App) startLiveSession(rect win.Rect) {
	a.overlay = nil
	a.stopLiveSession(false)

	ctx, cancel := context.WithCancel(context.Background())
	windowRect, barSide := computeFrameLayout(rect)
	initial := uidaemon.LivePayload{
		Anchor:     screenRect(rect),
		WindowRect: windowRect,
		BarSide:    barSide,
		Status:     "translating",
		Stage:      "ocr",
		Progress:   10,
	}
	a.liveMu.Lock()
	a.liveCancel = cancel
	a.liveActive = true
	a.livePaused = false
	a.liveDragging = false
	a.liveDragEpoch++
	a.liveRefresh = make(chan struct{}, 1)
	refresh := a.liveRefresh
	a.liveLast = initial
	a.liveRect = rect
	a.liveBarSide = barSide
	a.liveMu.Unlock()

	a.setStatus("实时翻译监听中")
	a.pushLive(initial)
	go a.liveMoveWatcher(ctx)
	go a.liveLoop(ctx, rect, refresh)
}

// liveLoop captures the selected region, skips frames that are visually
// identical to the last processed one, then runs OCR and translation. OCR and
// the LLM are deliberately serialized: both are CPU/memory-bandwidth bound, and
// overlapping them measured ~30% slower than running them one after the other.
func (a *App) liveLoop(ctx context.Context, rect win.Rect, refresh <-chan struct{}) {
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()

	var lastFingerprint win.FrameFingerprint
	haveFingerprint := false
	lastEpoch := -1
	forceNext := false
	for {
		force := forceNext
		forceNext = false
		if !force {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-refresh:
				force = true
			}
		}

		paused := a.isLivePaused()
		if paused && !force {
			continue
		}
		if a.isLiveDragging() {
			continue
		}

		// The user may drag or resize the live window at any time; derive the
		// monitored area from the window's current geometry.
		captureRect, windowRect, barSide, geometryOK := a.currentLiveGeometry()
		if !geometryOK {
			continue
		}
		epoch := a.liveEpoch()
		if epoch != lastEpoch {
			// A drag/resize invalidated the previous content; always recapture.
			lastEpoch = epoch
			haveFingerprint = false
		}

		startedAt := time.Now()
		// The overlay is excluded from capture (SetWindowDisplayAffinity), so it
		// can stay visible while we grab the screen. Only fall back to hiding it
		// when affinity is unavailable; hide/show flickers visibly.
		overlayWindow := win.FindWindowByTitle(overlayWindowTitle)
		captureExcluded := overlayWindow != 0 && win.WindowCaptureExcluded(overlayWindow)
		if overlayWindow != 0 && !captureExcluded {
			win.ShowWindow(overlayWindow, win.SWHide)
			time.Sleep(30 * time.Millisecond)
		}
		pngBytes, fingerprint, err := win.CaptureScreenFrame(captureRect)
		if overlayWindow != 0 && !captureExcluded {
			win.ShowWindow(overlayWindow, win.SWShowNoActivate)
		}
		if err != nil {
			log.Printf("app live capture failed: %v", err)
			continue
		}

		// Similar frames (caret blink, cursor movement, anti-aliasing noise) do
		// not change the recognized text, so reusing the previous overlay avoids
		// a full OCR + translation cycle. The refresh button bypasses this.
		if !force && haveFingerprint && lastFingerprint.Similar(fingerprint) {
			continue
		}
		haveFingerprint = true
		lastFingerprint = fingerprint
		a.pushLiveProgress("ocr", 15, 0)

		reqCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		var result backend.OCRTranslateBlocksResult
		var callErr error
		ocr, ocrErr := a.client.OCRBlocksWithLanguages(reqCtx, pngBytes, a.cfg.SourceLanguage)
		switch {
		case errors.Is(ocrErr, backend.ErrOCRBlocksUnsupported):
			log.Printf("app live ocr endpoint unsupported, using legacy combined path")
			a.pushLiveProgress("translate", 55, 0)
			result, callErr = a.ocrTranslateFrame(reqCtx, pngBytes)
		case ocrErr != nil:
			callErr = ocrErr
		default:
			a.pushLiveProgress("translate", 55, len(ocr.LineBoxes))
			result, callErr = a.translateOCRResult(reqCtx, ocr, force)
		}
		cancel()
		if ctx.Err() != nil {
			return
		}

		// A drag/resize during inference invalidates this frame: the watcher
		// already cleared the overlay, so never push stale blocks back.
		if a.liveEpoch() != epoch || a.isLiveDragging() {
			haveFingerprint = false
			continue
		}
		// If the window moved/resized while the engine was working, discard this
		// frame and immediately process the new region instead.
		if nextAnchor, _, _, nextOK := a.currentLiveGeometry(); nextOK && nextAnchor != captureRect {
			haveFingerprint = false
			continue
		}

		payload := a.livePayloadFrom(captureRect, windowRect, barSide, pngBytes, result, callErr)
		payload.LatencyMs = time.Since(startedAt).Milliseconds()
		if paused {
			payload.Status = "paused"
			payload.Stage = ""
			payload.Progress = 0
		}
		a.pushLive(payload)

		if callErr != nil {
			// Retry the same screen after the engine becomes available.
			haveFingerprint = false
		}
	}
}

// translateOCRResult translates an already-OCR'd frame, falling back to
// whole-text translation for engines without the split blocks endpoint.
// force bypasses the backend line cache (manual refresh).
func (a *App) translateOCRResult(ctx context.Context, ocr backend.OCRBlocksResult, force bool) (backend.OCRTranslateBlocksResult, error) {
	text := strings.TrimSpace(ocr.Text)
	if len(ocr.LineBoxes) > 0 {
		result, err := a.client.TranslateBlocksWithLanguages(
			ctx,
			ocr.LineBoxes,
			ocr.Text,
			ocr.ImageWidth,
			ocr.ImageHeight,
			a.cfg.SourceLanguage,
			a.cfg.TargetLanguage,
			force,
		)
		if !errors.Is(err, backend.ErrOCRBlocksUnsupported) {
			return result, err
		}
		log.Printf("translate_blocks endpoint unavailable, translating whole text")
	}
	if text == "" {
		// Blank region: clear the overlay quietly instead of showing an error.
		return backend.OCRTranslateBlocksResult{}, nil
	}
	translation, err := a.client.TranslateWithLanguages(
		ctx, text, a.cfg.SourceLanguage, a.cfg.TargetLanguage,
	)
	if err != nil {
		return backend.OCRTranslateBlocksResult{}, err
	}
	return backend.OCRTranslateBlocksResult{
		SourceText:  text,
		Translation: strings.TrimSpace(translation),
		ImageWidth:  ocr.ImageWidth,
		ImageHeight: ocr.ImageHeight,
	}, nil
}

// progressiveBatchLines keeps first results visible quickly for large pages.
// Each batch is one model call; the backend prompt prefix cache makes the
// per-batch overhead small.
const progressiveBatchLines = 4

func ocrBlocksFromLines(ocr backend.OCRBlocksResult) backend.OCRTranslateBlocksResult {
	result := backend.OCRTranslateBlocksResult{
		SourceText:  strings.TrimSpace(ocr.Text),
		ImageWidth:  ocr.ImageWidth,
		ImageHeight: ocr.ImageHeight,
	}
	for _, line := range ocr.LineBoxes {
		result.Blocks = append(result.Blocks, backend.OCRBlock{
			Text:       line.Text,
			X:          line.X,
			Y:          line.Y,
			W:          line.W,
			H:          line.H,
			Score:      line.Score,
			Angle:      line.Angle,
			CX:         line.CX,
			CY:         line.CY,
			LineWidth:  line.LineWidth,
			LineHeight: line.LineHeight,
		})
	}
	return result
}

// translateOCRProgressive translates OCR lines in small batches and calls
// onPartial after each batch. The full OCR geometry is available immediately,
// so the UI can show translated chips as soon as the first batch returns.
func (a *App) translateOCRProgressive(
	ctx context.Context,
	ocr backend.OCRBlocksResult,
	force bool,
	onPartial func(result backend.OCRTranslateBlocksResult, done, total int),
) (backend.OCRTranslateBlocksResult, error) {
	full := ocrBlocksFromLines(ocr)
	total := len(full.Blocks)
	if total == 0 {
		if text := strings.TrimSpace(ocr.Text); text != "" {
			translation, err := a.client.TranslateWithLanguages(
				ctx, text, a.cfg.SourceLanguage, a.cfg.TargetLanguage,
			)
			if err != nil {
				return full, err
			}
			full.Translation = strings.TrimSpace(translation)
		}
		return full, nil
	}

	var firstErr error
	for start := 0; start < total; start += progressiveBatchLines {
		if ctx.Err() != nil {
			return full, ctx.Err()
		}
		end := start + progressiveBatchLines
		if end > total {
			end = total
		}
		batch := make([]backend.OCRLine, 0, end-start)
		for _, block := range full.Blocks[start:end] {
			batch = append(batch, backend.OCRLine{
				Text:       block.Text,
				X:          block.X,
				Y:          block.Y,
				W:          block.W,
				H:          block.H,
				Score:      block.Score,
				Angle:      block.Angle,
				CX:         block.CX,
				CY:         block.CY,
				LineWidth:  block.LineWidth,
				LineHeight: block.LineHeight,
			})
		}
		translated, err := a.client.TranslateBlocksWithLanguages(
			ctx, batch, "", full.ImageWidth, full.ImageHeight,
			a.cfg.SourceLanguage, a.cfg.TargetLanguage, force,
		)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			for offset := range batch {
				if offset < len(translated.Blocks) {
					full.Blocks[start+offset].Translation = translated.Blocks[offset].Translation
				}
			}
		}
		if onPartial != nil {
			onPartial(full, end, total)
		}
	}

	parts := make([]string, 0, total)
	for _, block := range full.Blocks {
		if value := strings.TrimSpace(block.Translation); value != "" {
			parts = append(parts, value)
		}
	}
	full.Translation = strings.Join(parts, "\n")
	return full, firstErr
}

// currentLiveGeometry derives the monitored image area from the live window's
// current screen geometry. Dragging or resizing the window therefore changes
// the capture region immediately.
func (a *App) currentLiveGeometry() (win.Rect, win.Rect, string, bool) {
	a.liveMu.Lock()
	base := a.liveRect
	side := a.liveBarSide
	a.liveMu.Unlock()

	if base.Width() <= 0 || base.Height() <= 0 {
		return base, win.Rect{}, side, false
	}
	hwnd := win.FindWindowByTitle(overlayWindowTitle)
	if hwnd == 0 || !win.IsWindowVisible(hwnd) {
		return base, win.Rect{}, side, false
	}
	window, err := win.GetWindowRect(hwnd)
	if err != nil || window.Width() <= 0 || window.Height() <= 0 {
		return base, win.Rect{}, side, false
	}

	// The toolbar lives inside the window, so the capture area is the whole
	// window rectangle (the overlay itself is excluded from capture).
	return window, window, "top", true
}

func (a *App) livePayloadFrom(anchor win.Rect, window win.Rect, barSide string, pngBytes []byte, result backend.OCRTranslateBlocksResult, err error) uidaemon.LivePayload {
	if window.Width() <= 0 || window.Height() <= 0 {
		computed, computedSide := computeFrameLayout(anchor)
		window = win.Rect{
			Left:   int32(computed.X),
			Top:    int32(computed.Y),
			Right:  int32(computed.X + computed.Width),
			Bottom: int32(computed.Y + computed.Height),
		}
		barSide = computedSide
	}
	imageDataURL := ""
	if len(pngBytes) > 0 {
		if jpegBytes, jpegErr := win.EncodePNGToJPEG(pngBytes, 78); jpegErr == nil {
			imageDataURL = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpegBytes)
		}
	}
	payload := uidaemon.LivePayload{
		Anchor:       screenRect(anchor),
		WindowRect:   screenRect(window),
		BarSide:      barSide,
		ImageWidth:   int(anchor.Width()),
		ImageHeight:  int(anchor.Height()),
		ImageDataURL: imageDataURL,
		Source:       strings.TrimSpace(result.SourceText),
		Translation:  strings.TrimSpace(result.Translation),
		Status:       "listening",
		Style:        a.overlayStyle(),
	}
	for _, block := range result.Blocks {
		payload.Blocks = append(payload.Blocks, toTranslatedBlock(block))
	}
	clipBlockOverlaps(payload.Blocks)
	attachBlurredBackgrounds(pngBytes, payload.Blocks)
	if err != nil {
		payload.Status = "error"
		payload.Error = err.Error()
		payload.Stage = "error"
		payload.Progress = 0
	} else {
		payload.Stage = "done"
		payload.Progress = 100
		payload.Lines = len(payload.Blocks)
	}
	return payload
}

func sameWindowRect(a, b win.Rect) bool {
	return a.Left == b.Left && a.Top == b.Top && a.Right == b.Right && a.Bottom == b.Bottom
}

// clipBlockOverlaps shrinks nearly-horizontal line boxes so a line never covers
// the next one, even when OCR boxes touch or overlap in dense text. The
// original height is preserved in SourceHeight so the font size still matches
// the source text.
func clipBlockOverlaps(blocks []uidaemon.TranslatedBlock) {
	for index := range blocks {
		block := &blocks[index]
		if math.Abs(block.Angle) > 3 || block.SourceHeight <= 0 {
			continue
		}
		top := block.CY - block.LineHeight/2
		if block.LineHeight <= 0 {
			top = block.Y
		}
		bottom := top + block.SourceHeight
		limit := math.Inf(1)
		for otherIndex := range blocks {
			if otherIndex == index {
				continue
			}
			other := &blocks[otherIndex]
			if math.Abs(other.Angle) > 3 {
				continue
			}
			otherTop := other.CY - other.LineHeight/2
			if other.LineHeight <= 0 {
				otherTop = other.Y
			}
			if otherTop <= top+0.5 {
				continue
			}
			if block.X+block.W <= other.X || other.X+other.W <= block.X {
				continue
			}
			if otherTop < limit {
				limit = otherTop
			}
		}
		if math.IsInf(limit, 1) {
			continue
		}
		const gap = 1.0
		if bottom <= limit-gap {
			continue
		}
		newHeight := limit - gap - top
		if newHeight < 3 {
			newHeight = 3
		}
		if block.LineHeight > 0 {
			block.LineHeight = newHeight
			block.CY = top + newHeight/2
		}
		block.H = newHeight
	}
}

// pushLiveStatus updates only the status bubble while keeping the current
// payload, used when resuming after a drag.
func (a *App) pushLiveStatus(status string) {
	a.liveMu.Lock()
	payload := a.liveLast
	a.liveMu.Unlock()
	payload.Status = status
	payload.Stage = ""
	payload.Progress = 0
	a.pushLive(payload)
}

// pushLiveProgress shows the current recognition/translation stage while the
// existing blocks stay on screen.
func (a *App) pushLiveProgress(stage string, progress, lines int) {
	a.liveMu.Lock()
	payload := a.liveLast
	active := a.liveActive
	a.liveMu.Unlock()
	if !active {
		return
	}
	payload.Status = "translating"
	payload.Stage = stage
	payload.Progress = progress
	payload.Lines = lines
	payload.Error = ""
	a.pushLive(payload)
}

// clearLiveContent wipes every translated block so the image area becomes
// transparent again while the user drags/resizes the live window.
func (a *App) clearLiveContent() {
	a.liveMu.Lock()
	payload := a.liveLast
	a.liveMu.Unlock()
	payload.Blocks = nil
	payload.Source = ""
	payload.Translation = ""
	payload.Error = ""
	payload.LatencyMs = 0
	payload.Status = "paused"
	payload.Stage = ""
	payload.Progress = 0
	payload.Lines = 0
	a.pushLive(payload)
}

func (a *App) liveEpoch() int {
	a.liveMu.Lock()
	defer a.liveMu.Unlock()
	return a.liveDragEpoch
}

func (a *App) isLiveDragging() bool {
	a.liveMu.Lock()
	defer a.liveMu.Unlock()
	return a.liveDragging
}

func (a *App) isLivePaused() bool {
	a.liveMu.Lock()
	defer a.liveMu.Unlock()
	return a.livePaused
}

// beginLiveDrag marks a new drag and advances the epoch; it returns true only
// on the transition into dragging.
func (a *App) beginLiveDrag() bool {
	a.liveMu.Lock()
	defer a.liveMu.Unlock()
	if a.liveDragging {
		return false
	}
	a.liveDragging = true
	a.liveDragEpoch++
	return true
}

func (a *App) endLiveDrag() {
	a.liveMu.Lock()
	a.liveDragging = false
	a.liveMu.Unlock()
}

// liveMoveWatcher polls the overlay window rect quickly (60 ms) so a drag or
// resize pauses translation and blanks the overlay immediately, instead of
// waiting for the 700 ms capture loop. When the window has been stable for
// ~300 ms, capture resumes on the next loop tick.
func (a *App) liveMoveWatcher(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Millisecond)
	defer ticker.Stop()
	var last win.Rect
	stable := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		_, window, _, ok := a.currentLiveGeometry()
		if !ok {
			continue
		}
		if last.Width() > 0 && !sameWindowRect(window, last) {
			stable = 0
			if a.beginLiveDrag() {
				log.Printf("app live drag start: clearing overlay")
				a.clearLiveContent()
			}
		} else if a.isLiveDragging() {
			stable++
			if stable >= 5 {
				a.endLiveDrag()
				if !a.isLivePaused() {
					a.pushLiveStatus("listening")
				}
				stable = 0
			}
		}
		last = window
	}
}

func (a *App) pushLive(payload uidaemon.LivePayload) {
	a.liveMu.Lock()
	a.liveLast = payload
	a.liveMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.overlayUI.ShowLive(ctx, payload); err != nil {
		log.Printf("app show live panel failed: %v", err)
	}
}

func (a *App) setLivePaused(paused bool) {
	a.liveMu.Lock()
	if !a.liveActive {
		a.liveMu.Unlock()
		return
	}
	a.livePaused = paused
	if !paused {
		// Force the capture worker to re-grab the current screen: frames can
		// have been dropped while paused, so dedupe must not skip them.
		a.liveDragEpoch++
	}
	payload := a.liveLast
	payload.Stage = ""
	payload.Progress = 0
	payload.Lines = 0
	a.liveMu.Unlock()

	if paused {
		payload.Status = "paused"
		a.setStatus("实时翻译已暂停")
	} else {
		payload.Status = "listening"
		a.setStatus("实时翻译监听中")
	}
	payload.Error = ""
	a.pushLive(payload)
}

// requestLiveRefresh asks the live loop to re-run OCR and translation on the
// current screen even if it is visually identical to the previous frame.
func (a *App) requestLiveRefresh() {
	a.liveMu.Lock()
	refresh := a.liveRefresh
	active := a.liveActive
	a.liveMu.Unlock()
	if !active || refresh == nil {
		return
	}
	select {
	case refresh <- struct{}{}:
	default:
	}
	a.setStatus("正在刷新实时译文...")
}

func (a *App) stopLiveSession(hideUI bool) {
	a.liveMu.Lock()
	cancel := a.liveCancel
	active := a.liveActive
	a.liveCancel = nil
	a.liveActive = false
	a.livePaused = false
	a.liveRefresh = nil
	a.liveLast = uidaemon.LivePayload{}
	a.liveMu.Unlock()

	if cancel != nil {
		cancel()
	}
	// Make sure a capture interrupted mid-loop never leaves the window cloaked.
	if hwnd := win.FindWindowByTitle(overlayWindowTitle); hwnd != 0 {
		win.SetWindowCloaked(hwnd, false)
	}
	if !active {
		return
	}
	log.Printf("app live translation stopped")
	a.setStatus("就绪")
	if hideUI {
		ctx, cancelCtx := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelCtx()
		if err := a.ui.Hide(ctx); err != nil {
			log.Printf("app hide live panel failed: %v", err)
		}
	}
}

const (
	frameBarDip    = 44.0
	frameBorderDip = 2.0
)

// computeFrameLayout expands the selected image area by a toolbar strip and a
// border placed outside the image. The strip prefers the space above the
// selection and falls back below it, so it never covers translated text.
func computeFrameLayout(rect win.Rect) (uidaemon.ScreenRect, string) {
	// The overlay window is exactly the selected rectangle. The toolbar is drawn
	// inside it as a floating strip, so the selection never grows outside the
	// user's drag area.
	return uidaemon.ScreenRect{
		X:      int(rect.Left),
		Y:      int(rect.Top),
		Width:  int(rect.Width()),
		Height: int(rect.Height()),
	}, "top"
}

func (a *App) finishBusy() {
	a.resultMu.Lock()
	a.busy = false
	a.resultMu.Unlock()
}

func (a *App) pushResult(source, result string, latencyMs int64) {
	a.resultMu.Lock()
	a.source = source
	a.result = result
	a.lastErr = ""
	a.latencyMs = latencyMs
	a.resultMu.Unlock()
	log.Printf("app push result: source_chars=%d result_chars=%d", len(source), len(result))
	win.PostMessage(a.hwnd, wmTranslateDone, 0, 0)
}

func (a *App) pushError(source, errMsg string) {
	a.resultMu.Lock()
	a.source = source
	a.result = ""
	a.lastErr = errMsg
	a.latencyMs = 0
	a.resultMu.Unlock()
	log.Printf("app push error: source_chars=%d error=%q", len(source), errMsg)
	win.PostMessage(a.hwnd, wmTranslateError, 0, 0)
}

func (a *App) pushLoading(source string) {
	a.resultMu.Lock()
	a.source = source
	a.result = ""
	a.lastErr = ""
	a.latencyMs = 0
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

	liveText := "实时翻译"
	a.liveMu.Lock()
	if a.liveActive {
		liveText = "停止实时翻译"
	}
	a.liveMu.Unlock()

	items := []win.MenuItem{
		{ID: menuIDStatus, Text: "状态: " + a.status, Disabled: true},
		{ID: menuIDHotkey, Text: "翻译快捷键: " + a.cfg.HotkeyDisplay, Disabled: true},
		{ID: menuIDOCR, Text: "截图快捷键: " + a.cfg.OcrHotkeyDisplay, Disabled: true},
		{Separator: true},
		{ID: menuIDTranslate, Text: "文本翻译"},
		{ID: menuIDOCRRun, Text: "截图翻译（悬浮层）"},
		{ID: menuIDLive, Text: liveText},
		{ID: menuIDToggleOCR, Text: ocrToggleText},
		{Separator: true},
		{ID: menuIDExit, Text: "退出"},
	}
	win.ShowTrayMenu(a.hwnd, items)
}

func (a *App) openTranslate() {
	if err := a.showWailsTranslate(); err != nil {
		log.Printf("app show wails translate failed: %v", err)
		a.setStatus("翻译窗口启动失败")
		return
	}
	log.Printf("app show wails translate success")
}

func (a *App) showWailsTranslate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.ui.ShowTranslate(ctx); err != nil {
		return err
	}
	log.Printf("app wake ai engine after translate opened")
	go a.ensureBackendReady()
	return nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.ui.ShowSettings(ctx); err != nil {
		return err
	}
	// 设置弹窗会展示引擎状态徽标。若 AI 引擎正处于休眠（空闲自动回收），
	// 这里必须像翻译/OCR 一样主动拉起，否则徽标会一直停在“正在启动”的
	// 加载动画上，因为没有任何路径去唤醒引擎。
	log.Printf("app wake ai engine after settings opened")
	go a.ensureBackendReady()
	return nil
}

func (a *App) reloadSettings() {
	next, err := config.Load(a.baseDir)
	if err != nil {
		log.Printf("app reload settings failed: %v", err)
		a.pushError("", err.Error())
		return
	}
	if reflect.DeepEqual(next, a.cfg) {
		return
	}
	loggingChanged := next.LoggingEnabled != a.cfg.LoggingEnabled
	portChanged := next.ServerPort != a.cfg.ServerPort
	deviceChanged := next.AccelerationDevice != a.cfg.AccelerationDevice
	threadsChanged := next.AIThreads != a.cfg.AIThreads
	downloadChanged := next.ModelDownloadSource != a.cfg.ModelDownloadSource ||
		next.ModelDownloadBaseURL != a.cfg.ModelDownloadBaseURL
	// Adding or removing other catalogue entries must not restart a working
	// engine; only a change to the active model file does. A source-only change
	// is handled by needsDownload below.
	prevActive := config.ActiveModel(a.cfg)
	nextActive := config.ActiveModel(next)
	activeChanged := prevActive.ID != nextActive.ID ||
		prevActive.Repo != nextActive.Repo ||
		prevActive.File != nextActive.File ||
		prevActive.Dir != nextActive.Dir
	if err := a.applySettings(next); err != nil {
		log.Printf("app apply reloaded settings failed: %v", err)
		a.pushError("", err.Error())
		return
	}
	// The download source only matters when the model still has to be fetched;
	// do not reload an already working model just because the preference changed.
	needsDownload := (downloadChanged || activeChanged) && !config.ModelInstalled(a.baseDir, next)
	if loggingChanged || portChanged || deviceChanged || threadsChanged || activeChanged || needsDownload {
		go a.restartBackendForConfigChange(portChanged)
	}
}

func (a *App) applySettings(next config.Config) error {
	prev := a.cfg
	keepSuspended := a.hotkeysSuspended
	launchChanged := next.LaunchAtLogin != prev.LaunchAtLogin
	win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
	win.UnregisterHotKey(a.hwnd, hotkeyOCR)
	win.UnregisterHotKey(a.hwnd, hotkeyLive)

	a.cfg = next
	if err := a.registerHotkeys(); err != nil {
		// A hotkey can be owned by another application (or by a second PopTrans
		// instance). Keep the new non-hotkey settings, but restore the previous
		// hotkeys instead of rejecting the whole configuration as a
		// "translation failure".
		log.Printf("app hotkey registration failed, keeping previous hotkeys: %v", err)
		a.cfg.Hotkey = prev.Hotkey
		a.cfg.HotkeyDisplay = prev.HotkeyDisplay
		a.cfg.OcrHotkey = prev.OcrHotkey
		a.cfg.OcrHotkeyDisplay = prev.OcrHotkeyDisplay
		a.cfg.LiveHotkey = prev.LiveHotkey
		a.cfg.LiveHotkeyDisplay = prev.LiveHotkeyDisplay
		win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		win.UnregisterHotKey(a.hwnd, hotkeyLive)
		if restoreErr := a.registerHotkeys(); restoreErr != nil {
			log.Printf("app restore previous hotkeys failed (hotkeys unavailable): %v", restoreErr)
			a.setStatus("快捷键被占用，暂时无法注册")
		} else {
			log.Printf("app previous hotkeys restored")
		}
	}

	if err := config.Save(a.baseDir, a.cfg); err != nil {
		log.Printf("app save settings failed, restoring previous config: %v", err)
		win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		win.UnregisterHotKey(a.hwnd, hotkeyLive)
		a.cfg = prev
		_ = a.registerHotkeys()
		if keepSuspended {
			win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
			win.UnregisterHotKey(a.hwnd, hotkeyOCR)
			win.UnregisterHotKey(a.hwnd, hotkeyLive)
		}
		return err
	}

	if launchChanged {
		if err := a.syncLaunchAtLogin(); err != nil {
			log.Printf("app set launch at login failed, restoring previous config: %v", err)
			win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
			win.UnregisterHotKey(a.hwnd, hotkeyOCR)
			win.UnregisterHotKey(a.hwnd, hotkeyLive)
			a.cfg = prev
			_ = a.registerHotkeys()
			if keepSuspended {
				win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
				win.UnregisterHotKey(a.hwnd, hotkeyOCR)
				win.UnregisterHotKey(a.hwnd, hotkeyLive)
			}
			_ = config.Save(a.baseDir, prev)
			return fmt.Errorf("设置开机启动失败: %w", err)
		}
	}

	if !a.cfg.LiveEnabled {
		a.stopLiveSession(true)
	}
	if keepSuspended {
		win.UnregisterHotKey(a.hwnd, hotkeyTranslate)
		win.UnregisterHotKey(a.hwnd, hotkeyOCR)
		win.UnregisterHotKey(a.hwnd, hotkeyLive)
	}
	_ = applog.Configure(filepath.Join(a.baseDir, "translate.log"), a.cfg.LoggingEnabled)
	a.setBackendIdleTimeout(a.cfg.AIIdleTimeout())
	a.setStatus("设置已保存")
	return nil
}

// syncLaunchAtLogin writes or clears the HKCU Run entry so the app launches at
// login, according to the current config. The host process path (os.Executable)
// is used so the entry tracks the install location across directory moves.
func (a *App) syncLaunchAtLogin() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	if err := win.SetLaunchAtLogin("PopTrans", exePath, a.cfg.LaunchAtLogin); err != nil {
		return err
	}
	if a.cfg.LaunchAtLogin {
		log.Printf("app launch at login enabled: path=%s", exePath)
	} else {
		log.Printf("app launch at login disabled/cleared")
	}
	return nil
}

func (a *App) restartBackendForConfigChange(portChanged bool) {
	a.setStatus("正在重启本地服务...")
	a.cancelBackendIdleStop()
	ctx, cancel := context.WithTimeout(context.Background(), backend.StartupReadyTimeout)
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
	a.setBackendIdleTimeout(a.cfg.AIIdleTimeout())
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
		win.UnregisterHotKey(a.hwnd, hotkeyLive)
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

func (a *App) withBackend(ctx context.Context, onStatus func(string), fn func(context.Context) error) error {
	a.beginBackendUse()
	defer a.endBackendUse()

	if err := a.supervisor.EnsureRunning(ctx, onStatus); err != nil {
		return err
	}
	return fn(ctx)
}

func (a *App) beginBackendUse() {
	a.backendIdleMu.Lock()
	defer a.backendIdleMu.Unlock()

	a.backendIdleUsers++
	if a.backendIdleTimer != nil {
		a.backendIdleTimer.Stop()
		a.backendIdleTimer = nil
	}
}

func (a *App) endBackendUse() {
	a.backendIdleMu.Lock()
	if a.backendIdleUsers > 0 {
		a.backendIdleUsers--
	}
	shouldSchedule := a.backendIdleUsers == 0
	timeout := a.backendIdleTimeout
	if !shouldSchedule || timeout <= 0 {
		a.backendIdleMu.Unlock()
		if shouldSchedule && timeout <= 0 {
			log.Printf("backend idle auto-stop disabled")
		}
		return
	}
	if a.backendIdleTimer != nil {
		a.backendIdleTimer.Stop()
	}
	a.backendIdleTimer = time.AfterFunc(timeout, func() {
		a.stopBackendForIdleTimeout(timeout)
	})
	a.backendIdleMu.Unlock()
	log.Printf("backend idle auto-stop scheduled in %s", timeout)
}

func (a *App) cancelBackendIdleStop() {
	a.backendIdleMu.Lock()
	defer a.backendIdleMu.Unlock()

	if a.backendIdleTimer != nil {
		a.backendIdleTimer.Stop()
		a.backendIdleTimer = nil
	}
}

func (a *App) setBackendIdleTimeout(timeout time.Duration) {
	a.backendIdleMu.Lock()
	a.backendIdleTimeout = timeout
	idle := a.backendIdleUsers == 0
	if a.backendIdleTimer != nil {
		a.backendIdleTimer.Stop()
		a.backendIdleTimer = nil
	}
	if !idle || timeout <= 0 {
		a.backendIdleMu.Unlock()
		if timeout <= 0 {
			log.Printf("backend idle auto-stop disabled")
		}
		return
	}
	a.backendIdleTimer = time.AfterFunc(timeout, func() {
		a.stopBackendForIdleTimeout(timeout)
	})
	a.backendIdleMu.Unlock()
	log.Printf("backend idle auto-stop rescheduled in %s", timeout)
}

func (a *App) stopBackendForIdleTimeout(timeout time.Duration) {
	a.backendIdleMu.Lock()
	if a.backendIdleUsers != 0 {
		a.backendIdleTimer = nil
		a.backendIdleMu.Unlock()
		log.Printf("backend idle stop skipped: backend became busy again")
		return
	}
	a.backendIdleTimer = nil
	a.backendIdleMu.Unlock()

	log.Printf("backend idle timeout reached (%s), stopping ai engine", timeout)
	a.supervisor.Stop()
	a.setStatus("AI 引擎已休眠")
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
		a.setStatus("加载窗口启动失败")
		return
	}
	log.Printf("show loading wails popup success")
}

func (a *App) showWailsResult(source, result, errMsg string, loading bool) error {
	a.resultMu.Lock()
	latencyMs := a.latencyMs
	a.resultMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return a.ui.ShowResult(ctx, uidaemon.ResultPayload{
		Source:    source,
		Result:    result,
		Error:     errMsg,
		Loading:   loading,
		LatencyMs: latencyMs,
	})
}

func (a *App) handleUIDaemonEvent(event int) {
	switch event {
	case uidaemon.EventSettingsHidden, uidaemon.EventConfigSaved:
		win.PostMessage(a.hwnd, wmSettingsClosed, 0, 0)
	case uidaemon.EventUIExited:
		// Always re-post: resumeHotkeys is idempotent, and reading the flag on
		// this goroutine would race the window thread.
		win.PostMessage(a.hwnd, wmSettingsClosed, 0, 0)
	case uidaemon.EventHotkeyPause:
		win.PostMessage(a.hwnd, wmHotkeyPause, 0, 0)
	case uidaemon.EventHotkeyResume:
		win.PostMessage(a.hwnd, wmHotkeyResume, 0, 0)
	case uidaemon.EventOverlayClosed:
		win.PostMessage(a.hwnd, wmOverlayClosed, 0, 0)
	case uidaemon.EventOverlayRefresh:
		win.PostMessage(a.hwnd, wmOverlayRefresh, 0, 0)
	case uidaemon.EventLivePause:
		win.PostMessage(a.hwnd, wmLivePause, 0, 0)
	case uidaemon.EventLiveResume:
		win.PostMessage(a.hwnd, wmLiveResume, 0, 0)
	case uidaemon.EventLiveStop:
		win.PostMessage(a.hwnd, wmLiveStop, 0, 0)
	case uidaemon.EventLiveRefresh:
		win.PostMessage(a.hwnd, wmLiveRefresh, 0, 0)
	}
}

func (a *App) resolveWailsExe() string {
	if a.uiPath != "" {
		if _, err := os.Stat(a.uiPath); err == nil {
			return a.uiPath
		}
	}
	candidates := []string{
		filepath.Join(a.baseDir, runtimebundle.DirName, "translate-ui.exe"),
		filepath.Join(a.baseDir, "build", "bin", "translate-ui.exe"),
		filepath.Join(a.baseDir, "translate-ui.exe"),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "translate-ui.exe"))
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
		switch uint32(lParam) {
		case win.WmLButtonDblClk:
			log.Printf("main window proc: tray double click, open translate")
			currentApp.openTranslate()
		case win.WmRButtonUp:
			log.Printf("main window proc: tray right click, show tray menu")
			currentApp.showTrayMenu()
		}
		return 0
	case win.WmCommand:
		switch uint16(wParam & 0xFFFF) {
		case menuIDTranslate:
			currentApp.openTranslate()
			return 0
		case menuIDOCRRun:
			currentApp.handleOCRGo()
			return 0
		case menuIDLive:
			currentApp.handleLiveHotkey()
			return 0
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
			log.Printf("main window proc: hotkey ignored while settings ui is open")
			return 0
		}
		switch int32(wParam) {
		case hotkeyTranslate:
			currentApp.handleTranslate()
			return 0
		case hotkeyOCR:
			currentApp.handleOCRGo()
			return 0
		case hotkeyLive:
			currentApp.handleLiveHotkey()
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
		log.Printf("main window proc: ui settings closed")
		currentApp.reloadSettings()
		if err := currentApp.resumeHotkeys(); err != nil {
			log.Printf("resume hotkeys after wails settings closed: %v", err)
			currentApp.pushError("", err.Error())
		}
		return 0
	case wmHotkeyPause:
		log.Printf("main window proc: hotkey recording started")
		currentApp.suspendHotkeys()
		return 0
	case wmHotkeyResume:
		log.Printf("main window proc: hotkey recording finished")
		if err := currentApp.resumeHotkeys(); err != nil {
			log.Printf("resume hotkeys after recording: %v", err)
		}
		return 0
	case wmOverlayClosed:
		log.Printf("main window proc: screenshot overlay closed")
		currentApp.invalidateOverlay()
		currentApp.finishBusy()
		return 0
	case wmOverlayRefresh:
		log.Printf("main window proc: refresh screenshot translation")
		currentApp.refreshScreenshot()
		return 0
	case wmLivePause:
		log.Printf("main window proc: live translation paused")
		currentApp.setLivePaused(true)
		return 0
	case wmLiveResume:
		log.Printf("main window proc: live translation resumed")
		currentApp.setLivePaused(false)
		return 0
	case wmLiveStop:
		log.Printf("main window proc: live translation stopped by ui")
		currentApp.stopLiveSession(false)
		return 0
	case wmLiveRefresh:
		log.Printf("main window proc: refresh live translation")
		currentApp.requestLiveRefresh()
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

func hasArg(name string) bool {
	for _, arg := range os.Args[1:] {
		if arg == name {
			return true
		}
	}
	return false
}

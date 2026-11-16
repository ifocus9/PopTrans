package wailsui

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"translate-plugin/internal/backend"
	"translate-plugin/internal/config"
	win "translate-plugin/internal/platform/windows"
	"translate-plugin/internal/uidaemon"
)

const (
	eventUIState      = "ui:state"
	MainDefaultWidth  = 480
	MainDefaultHeight = 660
	MainMinWidth      = 400
	MainMinHeight     = 500
	maxOCRImageBytes  = 10 << 20
)

type App struct {
	ctx        context.Context
	baseDir    string
	client     *backend.Client
	supervisor *backend.Supervisor

	daemonMode    bool
	callbackURL   string
	callbackToken string
	notify        *uidaemon.NotifyClient
	server        *uidaemon.Server

	mu             sync.Mutex
	mode           string
	visible        bool
	settingsOpen   bool
	result         ResultView
	ready          bool
	startupLoading bool
	startupStatus  string
	startupError   string
	startupStarted time.Time
	idleTimer      *time.Timer
	idleTimeout    time.Duration
	mainWidth      int
	mainHeight     int

	clickOutsideMu     sync.Mutex
	clickOutsideCancel context.CancelFunc
}

type UIState struct {
	Config         config.Config  `json:"config"`
	Health         backend.Health `json:"health"`
	Mode           string         `json:"mode"`
	Result         ResultView     `json:"result"`
	StartupLoading bool           `json:"startup_loading"`
	StartupStatus  string         `json:"startup_status"`
	StartupError   string         `json:"startup_error"`
	SettingsOpen   bool           `json:"settings_open"`
	Visible        bool           `json:"visible"`
}

type TranslateResult struct {
	Source    string `json:"source"`
	Result    string `json:"result"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type OCRResult struct {
	Text      string `json:"text"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type ResultView struct {
	Source    string `json:"source"`
	Result    string `json:"result"`
	Error     string `json:"error"`
	Loading   bool   `json:"loading"`
	ElapsedMS int64  `json:"elapsed_ms,omitempty"`
}

// LaunchOptions configures translate-ui without passing secrets on the command line.
type LaunchOptions struct {
	Daemon        bool
	Mode          string
	Visible       bool
	SettingsOpen  bool
	CallbackURL   string
	CallbackToken string
}

// LaunchOptionsFromEnv reads daemon configuration from validated environment variables.
func LaunchOptionsFromEnv() LaunchOptions {
	var opts LaunchOptions
	opts.Daemon = os.Getenv("POPTRANS_UI_DAEMON") == "1"
	if raw := strings.TrimSpace(os.Getenv("POPTRANS_UI_CALLBACK_URL")); isLoopbackHTTPURL(raw) {
		opts.CallbackURL = raw
	}
	if token := strings.TrimSpace(os.Getenv("POPTRANS_UI_CALLBACK_TOKEN")); token != "" && len(token) <= 256 {
		opts.CallbackToken = token
	}
	return opts
}

// ApplyModeArgs applies non-secret display mode flags for manual standalone launches.
func (o *LaunchOptions) ApplyModeArgs(args []string) {
	for _, arg := range args {
		switch arg {
		case "--settings":
			o.Mode = "settings"
			o.Visible = true
			o.SettingsOpen = true
		case "--translate":
			o.Mode = "translate"
			o.Visible = true
			o.SettingsOpen = true
		case "--result":
			o.Mode = "result"
			o.Visible = true
		}
	}
}

func NewApp(baseDir string, launch LaunchOptions) *App {
	cfg, err := config.Load(baseDir)
	if err != nil {
		cfg = config.Default
	}
	client := backend.NewClient(config.BackendURL(cfg.ServerPort))
	mode := launch.Mode
	if mode == "" {
		mode = "settings"
	}
	app := &App{
		baseDir:       baseDir,
		client:        client,
		supervisor:    backend.NewSupervisor(baseDir, client),
		daemonMode:    launch.Daemon,
		mode:          mode,
		visible:       launch.Visible,
		settingsOpen:  launch.SettingsOpen,
		callbackURL:   launch.CallbackURL,
		callbackToken: launch.CallbackToken,
		idleTimeout:   cfg.UIIdleTimeout(),
		startupStatus: "正在启动本地服务...",
	}

	if app.callbackURL != "" && app.callbackToken != "" {
		app.notify = uidaemon.NewNotifyClient(app.callbackURL, app.callbackToken)
	}
	return app
}

// isLoopbackHTTPURL accepts only http://127.0.0.1[:port] or http://[::1][:port],
// matching the host process callback server.
func isLoopbackHTTPURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return false
	}
	ip := net.ParseIP(parsed.Hostname())
	return ip != nil && ip.IsLoopback()
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	if a.daemonMode {
		a.server = uidaemon.NewServer(a.baseDir, a)
		if _, err := a.server.Start(); err != nil {
			log.Printf("ui daemon server start failed: %v", err)
		}
		a.mu.Lock()
		a.ready = true
		a.mode = "translate"
		a.visible = true
		a.settingsOpen = false
		a.startupLoading = false
		a.startupStatus = ""
		a.startupError = ""
		a.startupStarted = time.Now()
		a.mu.Unlock()
		runtime.WindowSetMinSize(ctx, MainMinWidth, MainMinHeight)
		// The host process starts the backend. Poll its health here so the
		// non-blocking workbench badge/banner reflects download/load progress.
		go a.monitorStartup()
		return
	}

	// Non-daemon fallback (manual/dev launch).
	if a.mode == "result" {
		runtime.WindowSetMinSize(ctx, 320, 140)
		runtime.WindowSetMaxSize(ctx, 0, 0)
		positionResultWindow(ctx)
		a.mu.Lock()
		a.ready = true
		a.visible = true
		a.mu.Unlock()
		return
	}

	a.positionMainWindow()
	a.mu.Lock()
	a.ready = true
	if !a.visible {
		a.visible = true
		a.settingsOpen = true
	}
	a.mu.Unlock()
	go func() {
		startCtx, cancel := context.WithTimeout(context.Background(), backend.StartupReadyTimeout)
		defer cancel()
		_ = a.supervisor.EnsureRunning(startCtx, func(string) {})
	}()
}

func isStartupFailureStatus(status string) bool {
	if status == "" || strings.Contains(status, "正在重试") || strings.Contains(status, "断点续传") {
		return false
	}
	lower := strings.ToLower(status)
	return strings.Contains(status, "失败") ||
		strings.Contains(status, "拒绝") ||
		strings.Contains(status, "未找到") ||
		strings.Contains(status, "超时") ||
		strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed")
}

func (a *App) applyHealthToStartupLocked(health backend.Health) (ready bool) {
	if health.TranslatorReady {
		a.startupLoading = false
		a.startupStatus = "引擎就绪"
		a.startupError = ""
		return true
	}
	status := strings.TrimSpace(health.TranslatorStatus)
	if status == "" {
		status = "正在加载翻译模型..."
	}
	a.startupStatus = status
	if isStartupFailureStatus(status) {
		a.startupError = status
	} else {
		a.startupError = ""
	}
	return false
}

func (a *App) monitorStartup() {
	ctx := a.ctx
	if ctx == nil {
		return
	}
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(backend.StartupReadyTimeout)
	defer deadline.Stop()
	connFailCount := 0
	for {
		select {
		case <-ticker.C:
			healthCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			health, err := a.client.Health(healthCtx)
			cancel()

			a.mu.Lock()
			var ready bool
			if err != nil {
				connFailCount++
				a.startupStatus = "正在连接本地服务..."
				if connFailCount >= 20 {
					a.startupError = "无法连接本地 AI 服务"
				}
			} else {
				connFailCount = 0
				ready = a.applyHealthToStartupLocked(health)
			}
			a.mu.Unlock()
			a.emitState()
			if ready {
				return
			}
		case <-deadline.C:
			a.mu.Lock()
			a.startupLoading = false
			a.startupError = "等待模型加载超时，请检查网络或下载源设置"
			a.startupStatus = "模型加载超时"
			a.mu.Unlock()
			a.emitState()
			return
		case <-ctx.Done():
			return
		}
	}
}

func positionResultWindow(ctx context.Context) {
	const (
		windowWidth     = int32(425)
		maxWindowHeight = int32(440)
		cursorGap       = int32(18)
		screenMargin    = int32(12)
	)

	cursor := win.GetCursorPos()
	workArea := win.WorkAreaForPoint(cursor)
	x := cursor.X + cursorGap
	y := cursor.Y + cursorGap

	if x+windowWidth > workArea.Right-screenMargin {
		x = cursor.X - windowWidth - cursorGap
	}
	if y+maxWindowHeight > workArea.Bottom-screenMargin {
		y = cursor.Y - maxWindowHeight - cursorGap
	}
	if x < workArea.Left+screenMargin {
		x = workArea.Left + screenMargin
	}
	if y < workArea.Top+screenMargin {
		y = workArea.Top + screenMargin
	}

	// Wails 的 WindowSetPosition 把 (x,y) 当作相对窗口当前所在显示器工作区
	// 左上角的偏移(内部 workRect.Left + x),不是绝对屏幕坐标。直接传绝对
	// 坐标,在多显示器且窗口所在屏 workRect.Left != 0 时(如休眠唤醒后显示
	// 布局变化或窗口停在副屏)会叠加偏移把弹窗推到屏外;设置弹窗用
	// WindowCenter(绝对坐标)所以不受影响,这也解释了打开一次设置弹窗后
	// 翻译弹窗恢复正常。这里用窗口所在屏的 workArea.Left 补偿,使最终落点
	// 为绝对坐标 (x,y),并能跟随光标跨屏。
	winX, winY := runtime.WindowGetPosition(ctx)
	winArea := win.WorkAreaForPoint(win.Point{X: int32(winX), Y: int32(winY)})
	runtime.WindowSetPosition(ctx, int(x-winArea.Left), int(y-winArea.Top))
	runtime.WindowSetAlwaysOnTop(ctx, true)
}

func (a *App) captureMainWindowSize() {
	if a.ctx == nil {
		return
	}
	if runtime.WindowIsMaximised(a.ctx) || runtime.WindowIsMinimised(a.ctx) {
		return
	}
	w, h := runtime.WindowGetSize(a.ctx)
	if w >= MainMinWidth && h >= MainMinHeight {
		a.mu.Lock()
		a.mainWidth = w
		a.mainHeight = h
		a.mu.Unlock()
	}
}

func (a *App) positionMainWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
	runtime.WindowSetMinSize(a.ctx, MainMinWidth, MainMinHeight)
	runtime.WindowSetMaxSize(a.ctx, 0, 0)

	a.mu.Lock()
	w, h := a.mainWidth, a.mainHeight
	a.mu.Unlock()

	if w >= MainMinWidth && h >= MainMinHeight {
		if !runtime.WindowIsMaximised(a.ctx) {
			runtime.WindowSetSize(a.ctx, w, h)
		}
		return
	}
	runtime.WindowSetSize(a.ctx, MainDefaultWidth, MainDefaultHeight)
	runtime.WindowCenter(a.ctx)
}

func (a *App) Shutdown(ctx context.Context) {
	if a.server != nil {
		a.server.Stop()
	}
	if !a.daemonMode {
		a.supervisor.Stop()
	}
}

func (a *App) State() (UIState, error) {
	a.mu.Lock()
	mode := a.mode
	result := a.result
	a.mu.Unlock()

	cfg, err := config.Load(a.baseDir)
	if err != nil {
		return UIState{}, err
	}

	state := UIState{
		Config: cfg,
		Mode:   mode,
		Result: result,
	}
	a.mu.Lock()
	state.StartupLoading = a.startupLoading
	state.StartupStatus = a.startupStatus
	state.StartupError = a.startupError
	state.SettingsOpen = a.settingsOpen
	state.Visible = a.visible
	a.mu.Unlock()
	if mode == "settings" || mode == "translate" || state.StartupLoading {
		healthCtx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
		health, healthErr := a.client.Health(healthCtx)
		cancel()
		state.Health = health
		if healthErr == nil {
			a.mu.Lock()
			a.applyHealthToStartupLocked(health)
			state.StartupLoading = a.startupLoading
			state.StartupStatus = a.startupStatus
			state.StartupError = a.startupError
			a.mu.Unlock()
		}
	}
	return state, nil
}

func (a *App) SaveConfig(cfg config.Config) error {
	if err := config.ValidateServerPort(cfg.ServerPort); err != nil {
		return err
	}
	if err := config.Save(a.baseDir, cfg); err != nil {
		return err
	}
	a.notifyConfigSaved()
	return nil
}

func (a *App) Translate(text string) (TranslateResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return TranslateResult{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	startedAt := time.Now()
	result, err := a.client.Translate(ctx, text)
	if err != nil {
		return TranslateResult{}, err
	}
	return TranslateResult{
		Source:    text,
		Result:    strings.TrimSpace(result),
		ElapsedMS: time.Since(startedAt).Milliseconds(),
	}, nil
}

// OCRImage decodes a base64 image payload (or data URL), validates its format
// and 10MB size limit, and extracts text via the local OCR endpoint.
func (a *App) OCRImage(imageBase64 string) (OCRResult, error) {
	imgBytes, err := decodeBase64Image(imageBase64)
	if err != nil {
		return OCRResult{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	startedAt := time.Now()
	text, err := a.client.OCRImage(ctx, imgBytes)
	if err != nil {
		return OCRResult{}, err
	}
	return OCRResult{
		Text:      strings.TrimSpace(text),
		ElapsedMS: time.Since(startedAt).Milliseconds(),
	}, nil
}

func isSupportedImageBytes(data []byte) bool {
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return true
	}
	if len(data) >= 2 && data[0] == 'B' && data[1] == 'M' {
		return true
	}
	return strings.HasPrefix(http.DetectContentType(data), "image/")
}

func decodeBase64Image(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("图片内容为空")
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		idx := strings.Index(trimmed, ",")
		if idx < 0 {
			return nil, errors.New("无效的图片 Data URL")
		}
		meta := strings.ToLower(trimmed[:idx])
		if !strings.Contains(meta, ";base64") {
			return nil, errors.New("仅支持 base64 编码图片")
		}
		trimmed = trimmed[idx+1:]
	}
	if base64.StdEncoding.DecodedLen(len(trimmed)) > maxOCRImageBytes+3 {
		return nil, errors.New("图片大小不能超过 10MB")
	}
	data, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, errors.New("图片数据解码失败")
	}
	if len(data) == 0 {
		return nil, errors.New("图片内容为空")
	}
	if len(data) > maxOCRImageBytes {
		return nil, errors.New("图片大小不能超过 10MB")
	}
	if !isSupportedImageBytes(data) {
		return nil, errors.New("不支持的图片格式，请使用 PNG、JPG、WebP、BMP 或 GIF")
	}
	return data, nil
}

// HideWindow is called by the frontend close button in daemon mode.
func (a *App) HideWindow() error {
	return a.Hide()
}

// SetHotkeyRecording pauses or resumes global hotkeys while a settings hotkey
// input has focus so Windows does not intercept existing combinations.
func (a *App) SetHotkeyRecording(recording bool) error {
	if a.notify == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return a.notify.HotkeyHold(ctx, recording)
}

func (a *App) startClickOutsideMonitor() {
	a.clickOutsideMu.Lock()
	if a.clickOutsideCancel != nil {
		a.clickOutsideCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.clickOutsideCancel = cancel
	a.clickOutsideMu.Unlock()

	go a.monitorClickOutside(ctx)
}

func (a *App) stopClickOutsideMonitor() {
	a.clickOutsideMu.Lock()
	if a.clickOutsideCancel != nil {
		a.clickOutsideCancel()
		a.clickOutsideCancel = nil
	}
	a.clickOutsideMu.Unlock()
}

func (a *App) monitorClickOutside(ctx context.Context) {
	// 初始等待 250ms 防抖，过滤快捷键触发与初始挂载
	select {
	case <-ctx.Done():
		return
	case <-time.After(250 * time.Millisecond):
	}

	hwnd := win.FindWindowByTitle("选中翻译")
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	buttonWasUp := !win.IsMouseButtonDown()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.mu.Lock()
			visible := a.visible
			isResult := a.mode == "result"
			a.mu.Unlock()

			if !visible || !isResult {
				return
			}

			mouseDown := win.IsMouseButtonDown()
			if !mouseDown {
				buttonWasUp = true
				continue
			}

			// 如果不是新的一次按下（即用户还在持续按住拖拽/选择），直接跳过
			if !buttonWasUp {
				continue
			}

			// 消费本次按下事件（避免在按住拖拽过程中重复触发）
			buttonWasUp = false

			cursor := win.GetCursorPos()
			var inRect bool
			if hwnd != 0 {
				if rect, err := win.GetWindowRect(hwnd); err == nil {
					// 给予 10px 容差边缘，避免用户点击滚动条、阴影或拖拽边框时误判
					rect.Left -= 10
					rect.Top -= 10
					rect.Right += 10
					rect.Bottom += 10
					inRect = rect.Contains(cursor)
				}
			}
			if !inRect {
				winX, winY := runtime.WindowGetPosition(a.ctx)
				winW, winH := runtime.WindowGetSize(a.ctx)
				rect := win.Rect{
					Left:   int32(winX - 10),
					Top:    int32(winY - 10),
					Right:  int32(winX + winW + 10),
					Bottom: int32(winY + winH + 10),
				}
				inRect = rect.Contains(cursor)
			}

			if !inRect {
				log.Printf("monitorClickOutside: click outside detected at (%d, %d), closing result window", cursor.X, cursor.Y)
				_ = a.Hide()
				return
			}
		}
	}
}

// Status implements uidaemon.Handler.
func (a *App) Status() uidaemon.Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return uidaemon.Status{
		Ready:         a.ready,
		Mode:          a.mode,
		Visible:       a.visible,
		SettingsOpen:  a.settingsOpen,
		HotkeysPaused: a.settingsOpen,
	}
}

func (a *App) showMainMode(mode string) error {
	if a.ctx == nil {
		return context.Canceled
	}

	a.mu.Lock()
	a.mode = mode
	a.visible = true
	a.settingsOpen = true
	a.cancelIdleExitLocked()
	a.mu.Unlock()

	a.positionMainWindow()
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	a.emitState()
	return nil
}

// ShowSettings implements uidaemon.Handler.
func (a *App) ShowSettings() error {
	return a.showMainMode("settings")
}

// ShowTranslate implements uidaemon.Handler.
func (a *App) ShowTranslate() error {
	return a.showMainMode("translate")
}

// ShowResult implements uidaemon.Handler.
func (a *App) ShowResult(payload uidaemon.ResultPayload) error {
	if a.ctx == nil {
		return context.Canceled
	}

	a.mu.Lock()
	wasVisibleResult := a.visible && a.mode == "result"
	wasVisibleMain := a.visible && (a.mode == "settings" || a.mode == "translate")
	a.mu.Unlock()

	if wasVisibleMain {
		a.captureMainWindowSize()
	}

	a.mu.Lock()
	a.mode = "result"
	a.visible = true
	// Result can temporarily replace settings view; keep settingsOpen sticky so
	// host continues suspending hotkeys until settings are explicitly closed.
	a.result = ResultView{
		Source:    payload.Source,
		Result:    payload.Result,
		Error:     payload.Error,
		Loading:   payload.Loading,
		ElapsedMS: payload.ElapsedMS,
	}
	a.cancelIdleExitLocked()
	a.mu.Unlock()

	// Only place the popup on first show. Subsequent loading/result updates must
	// not chase the cursor or reset size, which causes a visible jump.
	if !wasVisibleResult {
		if runtime.WindowIsMaximised(a.ctx) {
			runtime.WindowUnmaximise(a.ctx)
		}
		runtime.WindowSetMinSize(a.ctx, 320, 140)
		runtime.WindowSetMaxSize(a.ctx, 0, 0)
		positionResultWindow(a.ctx)
		runtime.WindowSetSize(a.ctx, 425, 300)
		runtime.WindowUnminimise(a.ctx)
		hwnd := win.FindWindowByTitle("选中翻译")
		if hwnd != 0 {
			win.ForceForegroundWindow(hwnd)
		}
	}
	// Let the renderer show the window after DOM and icons have been committed;
	// otherwise the acrylic shell can appear a frame before the actual content.
	a.emitState()
	a.startClickOutsideMonitor()
	return nil
}

// Hide implements uidaemon.Handler.
func (a *App) Hide() error {
	a.stopClickOutsideMonitor()

	if a.ctx == nil {
		return nil
	}

	a.mu.Lock()
	wasSettings := a.settingsOpen || a.mode == "settings" || a.mode == "translate"
	prevMode := a.mode
	a.mu.Unlock()

	if prevMode == "settings" || prevMode == "translate" {
		a.captureMainWindowSize()
	}

	a.mu.Lock()
	a.visible = false
	a.settingsOpen = false
	if a.mode == "result" {
		a.result = ResultView{}
	}
	a.mode = "settings"
	a.mu.Unlock()

	runtime.WindowHide(a.ctx)
	a.scheduleIdleExit()
	if wasSettings {
		a.notifyHidden()
	}
	return nil
}

// Quit implements uidaemon.Handler.
func (a *App) Quit() error {
	a.mu.Lock()
	a.cancelIdleExitLocked()
	a.mu.Unlock()
	if a.server != nil {
		a.server.Stop()
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
		return nil
	}
	os.Exit(0)
	return nil
}

// OnHidden implements uidaemon.Handler for local server completeness.
func (a *App) OnHidden() {}

// OnConfigSaved implements uidaemon.Handler for local server completeness.
func (a *App) OnConfigSaved() {}

func (a *App) emitState() {
	if a.ctx == nil {
		return
	}
	state, err := a.State()
	if err != nil {
		log.Printf("emit ui state failed: %v", err)
		return
	}
	runtime.EventsEmit(a.ctx, eventUIState, state)
}

func (a *App) notifyHidden() {
	if a.notify == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.notify.Hidden(ctx); err != nil {
		log.Printf("notify hidden failed: %v", err)
	}
}

func (a *App) notifyConfigSaved() {
	if a.notify == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.notify.ConfigSaved(ctx); err != nil {
		log.Printf("notify config saved failed: %v", err)
	}
}

func (a *App) scheduleIdleExit() {
	if !a.daemonMode {
		return
	}

	timeout := a.idleTimeout
	if cfg, err := config.Load(a.baseDir); err == nil {
		timeout = cfg.UIIdleTimeout()
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.idleTimeout = timeout
	a.cancelIdleExitLocked()
	if a.idleTimeout <= 0 {
		log.Printf("ui idle auto-exit disabled")
		return
	}
	idleFor := a.idleTimeout
	a.idleTimer = time.AfterFunc(idleFor, func() {
		a.mu.Lock()
		visible := a.visible
		startupLoading := a.startupLoading
		a.mu.Unlock()
		if visible || startupLoading {
			log.Printf("ui idle exit skipped: window visible or startup loading in progress")
			return
		}
		log.Printf("ui idle timeout reached (%s), exiting daemon", idleFor)
		_ = a.Quit()
	})
	log.Printf("ui idle auto-exit scheduled in %s", idleFor)
}

func (a *App) cancelIdleExitLocked() {
	if a.idleTimer != nil {
		a.idleTimer.Stop()
		a.idleTimer = nil
	}
}

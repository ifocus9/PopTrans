package wailsui

import (
	"context"
	"log"
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
	eventUIState = "ui:state"
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
}

type TranslateResult struct {
	Source string `json:"source"`
	Result string `json:"result"`
}

type ResultView struct {
	Source  string `json:"source"`
	Result  string `json:"result"`
	Error   string `json:"error"`
	Loading bool   `json:"loading"`
}

func NewApp(baseDir string, args []string) *App {
	cfg, err := config.Load(baseDir)
	if err != nil {
		cfg = config.Default
	}
	client := backend.NewClient(config.BackendURL(cfg.ServerPort))
	app := &App{
		baseDir:       baseDir,
		client:        client,
		supervisor:    backend.NewSupervisor(baseDir, client),
		mode:          "settings",
		visible:       false,
		idleTimeout:   cfg.UIIdleTimeout(),
		startupStatus: "正在启动本地服务...",
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--daemon":
			app.daemonMode = true
		case "--callback-url":
			if i+1 < len(args) {
				app.callbackURL = args[i+1]
				i++
			}
		case "--callback-token":
			if i+1 < len(args) {
				app.callbackToken = args[i+1]
				i++
			}
		case "--settings":
			// Backward-compatible launch mode: open settings once.
			app.mode = "settings"
			app.visible = true
			app.settingsOpen = true
		case "--result":
			// Legacy one-shot result mode is no longer used by the host.
			// Keep parsing so old shortcuts do not crash.
			app.mode = "result"
			app.visible = true
			if i+1 < len(args) {
				i++
			}
		}
	}

	if app.callbackURL != "" && app.callbackToken != "" {
		app.notify = uidaemon.NewNotifyClient(app.callbackURL, app.callbackToken)
	}
	return app
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
		a.mode = "settings"
		a.visible = true
		a.settingsOpen = false
		a.startupLoading = true
		a.startupStarted = time.Now()
		a.mu.Unlock()
		// The host process starts the backend. Poll its health here so the UI can
		// show download/load progress without racing a second backend process.
		go a.monitorStartup()
		return
	}

	// Non-daemon fallback (manual/dev launch).
	if a.mode == "result" {
		positionResultWindow(ctx)
		a.mu.Lock()
		a.ready = true
		a.visible = true
		a.mu.Unlock()
		return
	}

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

func (a *App) monitorStartup() {
	ctx := a.ctx
	if ctx == nil {
		return
	}
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(backend.StartupReadyTimeout)
	defer deadline.Stop()
	for {
		select {
		case <-ticker.C:
			health, err := a.client.Health(ctx)
			a.mu.Lock()
			if err != nil {
				a.startupStatus = "正在连接本地服务..."
			} else if health.TranslatorStatus != "" {
				a.startupStatus = health.TranslatorStatus
			} else if !health.TranslatorReady {
				a.startupStatus = "正在加载翻译模型..."
			}
			ready := err == nil && health.TranslatorReady && time.Since(a.startupStarted) >= 1200*time.Millisecond
			// 冷启动由本函数收尾：模型就绪后隐藏启动窗口。但 daemon 可能因
			// UI 空闲退出后被宿主首个快捷键/设置请求重新拉起，此时宿主已把
			// 视图切到 result 弹窗(ShowResult)或打开设置(ShowSettings)。
			// 若这里无条件 WindowHide，会藏掉正在显示的翻译/加载弹窗，且 Go
			// 侧 visible=true 与真实窗口不可见不一致——这就是"AI 引擎休眠后
			// 首个翻译弹窗看不到"的根因。仅纯冷启动(settings 未打开)才收尾。
			mode := a.mode
			settingsOpen := a.settingsOpen
			if ready {
				a.startupLoading = false
				a.startupStatus = "模型加载完成"
			}
			if ready && mode == "settings" && !settingsOpen {
				a.visible = false
			}
			a.mu.Unlock()
			a.emitState()
			if ready {
				if mode == "settings" && !settingsOpen {
					runtime.WindowHide(ctx)
				}
				a.scheduleIdleExit()
				return
			}
		case <-deadline.C:
			a.mu.Lock()
			// Keep the startup page visible on timeout so the user can see why the
			// app did not move to the tray.
			a.startupLoading = true
			a.startupError = "等待模型加载超时，请检查网络或模型文件"
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

func positionSettingsWindow(ctx context.Context) {
	runtime.WindowSetAlwaysOnTop(ctx, false)
	runtime.WindowSetSize(ctx, 458, 640)
	runtime.WindowCenter(ctx)
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
	a.mu.Unlock()
	if mode == "settings" || state.StartupLoading {
		healthCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		health, _ := a.client.Health(healthCtx)
		cancel()
		state.Health = health
	}
	return state, nil
}

func (a *App) SaveConfig(cfg config.Config) error {
	if err := config.ValidateServerPort(cfg.ServerPort); err != nil {
		return err
	}
	// Host reloads settings when the settings window is hidden.
	return config.Save(a.baseDir, cfg)
}

func (a *App) Translate(text string) (TranslateResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return TranslateResult{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	result, err := a.client.Translate(ctx, text)
	if err != nil {
		return TranslateResult{}, err
	}
	return TranslateResult{Source: text, Result: result}, nil
}

// HideWindow is called by the frontend close button in daemon mode.
func (a *App) HideWindow() {
	_ = a.Hide()
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

// ShowSettings implements uidaemon.Handler.
func (a *App) ShowSettings() error {
	if a.ctx == nil {
		return context.Canceled
	}

	a.mu.Lock()
	a.mode = "settings"
	a.visible = true
	a.settingsOpen = true
	a.cancelIdleExitLocked()
	a.mu.Unlock()

	positionSettingsWindow(a.ctx)
	a.emitState()
	return nil
}

// ShowResult implements uidaemon.Handler.
func (a *App) ShowResult(payload uidaemon.ResultPayload) error {
	if a.ctx == nil {
		return context.Canceled
	}

	a.mu.Lock()
	wasVisibleResult := a.visible && a.mode == "result"
	a.mode = "result"
	a.visible = true
	// Result can temporarily replace settings view; keep settingsOpen sticky so
	// host continues suspending hotkeys until settings are explicitly closed.
	a.result = ResultView{
		Source:  payload.Source,
		Result:  payload.Result,
		Error:   payload.Error,
		Loading: payload.Loading,
	}
	a.cancelIdleExitLocked()
	a.mu.Unlock()

	// Only place the popup on first show. Subsequent loading/result updates must
	// not chase the cursor or reset size, which causes a visible jump.
	if !wasVisibleResult {
		positionResultWindow(a.ctx)
		runtime.WindowSetSize(a.ctx, 425, 300)
	}
	// Let the renderer show the window after DOM and icons have been committed;
	// otherwise the acrylic shell can appear a frame before the actual content.
	a.emitState()
	return nil
}

// Hide implements uidaemon.Handler.
func (a *App) Hide() error {
	if a.ctx == nil {
		return nil
	}

	wasSettings := false
	a.mu.Lock()
	wasSettings = a.settingsOpen || a.mode == "settings"
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
		a.mu.Unlock()
		if visible {
			log.Printf("ui idle exit skipped: window visible again")
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

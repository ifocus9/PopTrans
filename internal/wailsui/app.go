package wailsui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"translate-plugin/internal/backend"
	"translate-plugin/internal/config"
	"translate-plugin/internal/modelhub"
	win "translate-plugin/internal/platform/windows"
	"translate-plugin/internal/uidaemon"
)

const (
	eventUIState       = "ui:state"
	eventOverlay       = "ui:overlay"
	eventLive          = "ui:live"
	eventModelDownload = "ui:model-download"
)

type App struct {
	ctx        context.Context
	baseDir    string
	client     *backend.Client
	supervisor *backend.Supervisor

	daemonMode    bool
	overlayMode   bool
	endpointDir   string
	windowTitle   string
	callbackURL   string
	callbackToken string
	notify        *uidaemon.NotifyClient
	server        *uidaemon.Server

	mu             sync.Mutex
	mode           string
	visible        bool
	settingsOpen   bool
	mainSized      bool
	result         ResultView
	ready          bool
	startupLoading bool
	startupStatus  string
	startupError   string
	startupStarted time.Time
	idleTimer      *time.Timer
	idleTimeout    time.Duration

	clickOutsideMu     sync.Mutex
	clickOutsideCancel context.CancelFunc

	downloadMu     sync.Mutex
	downloadCancel context.CancelFunc
	downloading    bool

	// Screenshot overlay / live panel state. Overlay payloads are emitted
	// through dedicated events instead of the polled UIState.
	overlayRect uidaemon.ScreenRect
	liveRect    uidaemon.ScreenRect

	hitMu            sync.Mutex
	hitTestRect      win.Rect
	hitTestActive    bool
	hitTestResizable bool
	overlayPrepared  bool
	regionSig        string
	origWndProc      uintptr
	// overlaySizing lets an overlay window use its selection-derived size
	// instead of the workbench minimum. The responsive toolbar keeps the "…"
	// and close buttons usable down to the small overlay floor.
	overlaySizing bool
	// overlayLive tracks whether the current overlay is the live panel. Both
	// modes share the same 56x1 floor; live simply carries one extra action.
	overlayLive bool

	pinMu         sync.Mutex
	overlayPinned bool
}

// hitTestApp is the single overlay-process App whose window is subclassed for
// click-through hit testing.
var hitTestApp *App

const (
	// Overlay windows are resized with physical pixels by the host. Both modes
	// use the same small floor because the responsive toolbar folds every
	// non-essential action into the "…" menu; the live panel adds a pause button
	// but never forces a wider selection.
	overlayMinWidthDIP  = 56
	overlayMinHeightDIP = 1
	liveMinWidthDIP     = 56
	liveMinHeightDIP    = 1

	wmShowOverlayMenu = win.WmApp + 40
)

// overlayMenuRequest is consumed synchronously by the overlay window's UI
// thread. TrackPopupMenu must run on the thread that owns the window, otherwise
// it returns immediately without showing anything.
type overlayMenuRequest struct {
	x        int32
	y        int32
	below    bool
	items    []win.MenuItem
	selected uint16
}

var (
	overlayMenuMu  sync.Mutex
	overlayMenuReq *overlayMenuRequest
)

var overlayHitTestProc = syscall.NewCallback(func(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	app := hitTestApp
	if msg == win.WmMeasureItem {
		if win.HandleOwnerDrawMeasure(lParam) {
			return 1
		}
	}
	if msg == win.WmDrawItem {
		if win.HandleOwnerDrawDraw(lParam) {
			return 1
		}
	}
	if msg == wmShowOverlayMenu {
		if request := overlayMenuReq; request != nil {
			started := time.Now()
			request.selected = win.ShowPopupMenuAt(
				win.HWND(hwnd), request.x, request.y, request.below, request.items,
			)
			// The first click can race the WebView's mouse-up handling and
			// immediately cancel the modal menu. Retry once only when the menu
			// returned instantly, which the user could not have dismissed.
			if request.selected == 0 && time.Since(started) < 120*time.Millisecond {
				time.Sleep(120 * time.Millisecond)
				request.selected = win.ShowPopupMenuAt(
					win.HWND(hwnd), request.x, request.y, request.below, request.items,
				)
			}
		}
		return 0
	}
	if app != nil && msg == win.WmGetMinMaxInfo && app.overlaySizingEnabled() {
		// Wails' workbench minimum (420x560) would otherwise expand small
		// selections. Keep the overlay's own control floor, scaled to the
		// monitor DPI because MINMAXINFO uses physical pixels.
		dpi := win.DpiForWindow(win.HWND(hwnd))
		widthDIP, heightDIP := app.overlayMinDIP()
		width := int32((widthDIP*int(dpi) + 95) / 96)
		height := int32((heightDIP*int(dpi) + 95) / 96)
		win.SetMinTrackSize(lParam, width, height)
		return 0
	}
	if app != nil && msg == win.WmNcHitTest {
		if app.hitTestEnabled() {
			screen := win.LParamPoint(lParam)
			client := win.ScreenToClient(win.HWND(hwnd), screen)

			// Edge resizing is driven from the page: returning HTLEFT/HTTOP
			// here would steal the pointerdown from the WebView edge strips.
			if app.hitRect().Contains(client) {
				return win.HtTransparent
			}
		}
	}
	if app != nil && app.origWndProc != 0 {
		return win.CallWindowProc(app.origWndProc, win.HWND(hwnd), msg, wParam, lParam)
	}
	return win.DefWindowProc(win.HWND(hwnd), msg, wParam, lParam)
})

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
	LatencyMs int64  `json:"latency_ms"`
}

type ResultView struct {
	Source    string `json:"source"`
	Result    string `json:"result"`
	Error     string `json:"error"`
	Loading   bool   `json:"loading"`
	LatencyMs int64  `json:"latency_ms"`
}

// LaunchOptions are the launch settings for the UI process. The host passes
// them through the environment so paths and the callback token never appear on
// the process command line.
type LaunchOptions struct {
	Daemon        bool
	Overlay       bool
	EndpointDir   string
	CallbackURL   string
	CallbackToken string
}

// LaunchOptionsFromEnv reads and validates the UI process environment. Only an
// absolute endpoint directory and a loopback callback URL are accepted.
func LaunchOptionsFromEnv() LaunchOptions {
	var opts LaunchOptions
	opts.Daemon = os.Getenv("POPTRANS_UI_DAEMON") == "1"
	opts.Overlay = os.Getenv("POPTRANS_UI_OVERLAY") == "1"
	if dir := strings.TrimSpace(os.Getenv("POPTRANS_UI_ENDPOINT_DIR")); dir != "" {
		if cleaned := filepath.Clean(dir); filepath.IsAbs(cleaned) {
			opts.EndpointDir = cleaned
		}
	}
	if raw := strings.TrimSpace(os.Getenv("POPTRANS_UI_CALLBACK_URL")); isLoopbackHTTPURL(raw) {
		opts.CallbackURL = raw
	}
	if token := strings.TrimSpace(os.Getenv("POPTRANS_UI_CALLBACK_TOKEN")); token != "" && len(token) <= 256 {
		opts.CallbackToken = token
	}
	return opts
}

func NewApp(baseDir string, launch LaunchOptions) *App {
	cfg, err := config.Load(baseDir)
	if err != nil {
		cfg = config.Default
	}
	client := backend.NewClient(config.BackendURL(cfg.ServerPort))
	mode := "settings"
	if launch.Overlay {
		mode = "overlay"
	}
	app := &App{
		baseDir:       baseDir,
		client:        client,
		supervisor:    backend.NewSupervisor(baseDir, client),
		endpointDir:   launch.EndpointDir,
		windowTitle:   "选中翻译",
		mode:          mode,
		daemonMode:    launch.Daemon,
		overlayMode:   launch.Overlay,
		callbackURL:   launch.CallbackURL,
		callbackToken: launch.CallbackToken,
		idleTimeout:   cfg.UIIdleTimeout(),
		startupStatus: "正在启动本地服务...",
		overlayPinned: true,
	}
	if app.mode == "" {
		app.mode = "settings"
	}
	if app.overlayMode {
		app.windowTitle = "选中翻译浮层"
	}
	if app.endpointDir == "" {
		app.endpointDir = baseDir
	}
	if app.callbackURL != "" && app.callbackToken != "" {
		app.notify = uidaemon.NewNotifyClient(app.callbackURL, app.callbackToken)
	}
	return app
}

// isLoopbackHTTPURL accepts only http://127.0.0.1:<port> (or another loopback
// address), matching the host's callback server.
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
		a.server = uidaemon.NewServerWithEndpointDir(a.baseDir, a.endpointDir, a)
		if _, err := a.server.Start(); err != nil {
			log.Printf("ui daemon server start failed: %v", err)
		}
		if a.overlayMode {
			// Dedicated overlay/live renderer: no workbench, no startup polling.
			a.mu.Lock()
			a.ready = true
			a.mode = "overlay"
			a.visible = false
			a.settingsOpen = false
			a.startupLoading = false
			a.mu.Unlock()
			go a.applyWindowAppearance()
			go a.monitorClickThrough(ctx)
			return
		}
		a.mu.Lock()
		a.ready = true
		a.mode = "translate"
		a.visible = true
		a.settingsOpen = false
		// The workbench is usable immediately. Model/engine startup continues in
		// the background and is surfaced through the header badge and banner; it
		// must never block settings, so the user can change the download source
		// when the default mirror is unreachable.
		a.startupLoading = false
		a.startupStatus = ""
		a.startupError = ""
		a.startupStarted = time.Now()
		a.mu.Unlock()
		// The host process starts the backend. Poll its health here so the UI can
		// show download/load progress without racing a second backend process.
		go a.monitorStartup()
		go a.applyWindowAppearance()
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

// IsOverlayProcess tells the frontend whether this process renders only the
// screenshot/live overlay (so it never builds the workbench UI).
func (a *App) IsOverlayProcess() bool {
	return a.overlayMode
}

// applyWindowAppearance gives both windows the product icon and keeps the
// overlay window out of the taskbar/Alt-Tab list.
func (a *App) applyWindowAppearance() {
	iconPath := filepath.Join(a.baseDir, "icon.ico")
	if _, err := os.Stat(iconPath); err != nil {
		iconPath = filepath.Join(a.baseDir, "assets", "icon.ico")
	}
	for attempt := 0; attempt < 60; attempt++ {
		if hwnd := win.FindWindowByTitle(a.windowTitle); hwnd != 0 {
			a.prepareOverlayWindow()
			win.SetWindowIconFromFile(hwnd, iconPath)
			// The single-exe release has no icon.ico next to the app; use the
			// icon resource compiled into this executable instead.
			win.SetWindowIconFromResource(hwnd)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	log.Printf("window appearance: window %q not found", a.windowTitle)
}

// prepareOverlayWindow keeps the overlay renderer out of the taskbar, clears
// the acrylic backdrop so live translations are transparent, and installs the
// click-through hit test.
func (a *App) prepareOverlayWindow() {
	if !a.overlayMode {
		return
	}
	hwnd := win.FindWindowByTitle(a.windowTitle)
	if hwnd == 0 {
		return
	}
	a.hitMu.Lock()
	prepared := a.overlayPrepared
	a.hitMu.Unlock()
	if prepared {
		return
	}
	win.MakeToolWindow(hwnd)
	win.RemoveWindowFrame(hwnd)
	// Overlay frames report a phantom client height equal to the window height
	// in one-pixel-overlap cases (e.g. 96x44 window, 96 client height). Let the
	// WebView fill the real client area so the page's innerHeight always
	// matches the physical window it is laid out for.
	win.FillClientArea(hwnd)
	if a.origWndProc == 0 {
		hitTestApp = a
		a.origWndProc = win.SubclassWindow(hwnd, overlayHitTestProc)
	}
	a.hitMu.Lock()
	a.overlayPrepared = true
	a.hitMu.Unlock()
}

// setCaptureVisibility keeps the overlay visible to Snipping Tool and other
// screenshot apps. During the host's own live frame grab, SetCaptureGuard
// briefly excludes it and then restores normal capture.
func (a *App) setCaptureVisibility(hwnd win.HWND, allow bool) {
	if hwnd == 0 {
		return
	}
	if os.Getenv("POPTRANS_ALLOW_CAPTURE") == "1" {
		allow = true
	}
	if allow {
		if !win.SetWindowCaptureAllowed(hwnd) {
			log.Printf("overlay: failed to allow screen capture")
		}
		return
	}
	if !win.SetWindowCaptureExcluded(hwnd) {
		log.Printf("overlay: SetWindowDisplayAffinity failed; live capture guard unavailable")
	}
}

// SetCaptureGuard implements uidaemon.Handler. It briefly hides the overlay
// from screen capture while the host grabs a live frame, then restores normal
// capture so Snipping Tool and other apps can always see the panel.
func (a *App) SetCaptureGuard(exclude bool) error {
	hwnd := win.FindWindowByTitle(a.windowTitle)
	if hwnd == 0 {
		return nil
	}
	if exclude {
		if !win.SetWindowCaptureExcluded(hwnd) {
			return fmt.Errorf("exclude overlay from capture")
		}
		return nil
	}
	if !win.SetWindowCaptureAllowed(hwnd) {
		return fmt.Errorf("allow overlay capture")
	}
	return nil
}

// monitorClickThrough keeps the overlay content area click-through. WebView2
// renders inside a child window, so a WM_NCHITTEST subclass on the top-level
// window cannot swallow mouse input for the rendered chips. Polling the cursor
// lets us toggle WS_EX_TRANSPARENT exactly while it is over the image area.
func (a *App) monitorClickThrough(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()
	var hwnd win.HWND
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if hwnd == 0 || !win.IsWindowVisible(hwnd) {
			hwnd = win.FindWindowByTitle(a.windowTitle)
		}
		if hwnd == 0 {
			continue
		}
		if !win.IsWindowVisible(hwnd) || !a.hitTestEnabled() {
			win.SetClickThrough(hwnd, false)
			continue
		}

		cursor := win.ScreenToClient(hwnd, win.GetCursorPos())
		interactive := !a.hitRect().Contains(cursor)
		if a.hitTestEdgeResizable() {
			// Leave the resize border interactive so dragging an edge can
			// change the monitored region.
			const edge = int32(8)
			client := win.GetClientRect(hwnd)
			if cursor.X < edge || cursor.Y < edge ||
				cursor.X >= client.Right-edge || cursor.Y >= client.Bottom-edge {
				interactive = true
			}
		}
		win.SetClickThrough(hwnd, !interactive)
	}
}

// SetOverlayWindowRect moves/resizes the overlay window while the user drags
// one of its edges. Values arrive in CSS pixels; Win32 works in physical pixels.
func (a *App) SetOverlayWindowRect(x, y, w, h int) {
	if !a.overlayMode || w <= 0 || h <= 0 {
		return
	}
	hwnd := win.FindWindowByTitle(a.windowTitle)
	if hwnd == 0 {
		return
	}
	current, err := win.GetWindowRect(hwnd)
	if err != nil {
		return
	}
	scale := float64(win.DpiForRect(current)) / 96.0
	if scale <= 0 {
		scale = 1
	}
	win.MoveWindow(
		hwnd,
		int32(float64(x)*scale),
		int32(float64(y)*scale),
		int32(float64(w)*scale),
		int32(float64(h)*scale),
		true,
	)
}

// SetOverlayRegion applies the window shape the overlay page computed from its
// rendered toolbar/chips. The JSON array uses physical client pixels; areas
// outside it become invisible and click-through.
func (a *App) SetOverlayRegion(rectsJSON string) {
	if !a.overlayMode {
		return
	}
	a.hitMu.Lock()
	same := a.regionSig == rectsJSON
	a.hitMu.Unlock()
	if same {
		// Re-applying an identical window region makes DWM repaint the shaped
		// window; skip it so live updates don't flash.
		return
	}
	var raw []struct {
		X int `json:"x"`
		Y int `json:"y"`
		W int `json:"w"`
		H int `json:"h"`
	}
	var request struct {
		Rects []struct {
			X int `json:"x"`
			Y int `json:"y"`
			W int `json:"w"`
			H int `json:"h"`
		} `json:"rects"`
		Polys [][][2]int `json:"polys"`
	}
	if err := json.Unmarshal([]byte(rectsJSON), &request); err == nil &&
		(len(request.Rects) > 0 || len(request.Polys) > 0 || strings.HasPrefix(strings.TrimSpace(rectsJSON), "{")) {
		raw = request.Rects
	} else if err := json.Unmarshal([]byte(rectsJSON), &raw); err != nil {
		log.Printf("overlay region decode failed: %v", err)
		return
	}
	hwnd := win.FindWindowByTitle(a.windowTitle)
	if hwnd == 0 {
		return
	}
	a.hitMu.Lock()
	a.regionSig = rectsJSON
	a.hitMu.Unlock()
	if len(raw) == 0 && len(request.Polys) == 0 {
		win.SetWindowRegion(hwnd, nil)
		return
	}
	rects := make([]win.Rect, 0, len(raw))
	for _, item := range raw {
		rects = append(rects, win.Rect{
			Left:   int32(item.X),
			Top:    int32(item.Y),
			Right:  int32(item.X + item.W),
			Bottom: int32(item.Y + item.H),
		})
	}
	polys := make([][]win.Point, 0, len(request.Polys))
	for _, poly := range request.Polys {
		points := make([]win.Point, 0, len(poly))
		for _, point := range poly {
			points = append(points, win.Point{X: int32(point[0]), Y: int32(point[1])})
		}
		if len(points) >= 3 {
			polys = append(polys, points)
		}
	}
	win.SetWindowRegionShapes(hwnd, rects, polys)
}

func (a *App) hitTestEnabled() bool {
	a.hitMu.Lock()
	defer a.hitMu.Unlock()
	return a.hitTestActive
}

func (a *App) setOverlaySizing(allow bool) {
	a.hitMu.Lock()
	a.overlaySizing = allow
	a.hitMu.Unlock()
}

func (a *App) overlaySizingEnabled() bool {
	a.hitMu.Lock()
	defer a.hitMu.Unlock()
	return a.overlaySizing
}

func (a *App) overlayMinDIP() (int, int) {
	a.hitMu.Lock()
	live := a.overlayLive
	a.hitMu.Unlock()
	if live {
		return liveMinWidthDIP, liveMinHeightDIP
	}
	return overlayMinWidthDIP, overlayMinHeightDIP
}

func (a *App) hitRect() win.Rect {
	a.hitMu.Lock()
	defer a.hitMu.Unlock()
	return a.hitTestRect
}

func (a *App) hitTestEdgeResizable() bool {
	a.hitMu.Lock()
	defer a.hitMu.Unlock()
	return a.hitTestActive && a.hitTestResizable
}

func (a *App) setHitTest(rect win.Rect, active, resizable bool) {
	a.hitMu.Lock()
	a.hitTestRect = rect
	a.hitTestActive = active
	a.hitTestResizable = resizable
	a.hitMu.Unlock()
}

func (a *App) isOverlayPinned() bool {
	a.pinMu.Lock()
	defer a.pinMu.Unlock()
	return a.overlayPinned
}

func (a *App) setOverlayPinned(pinned bool) {
	a.pinMu.Lock()
	a.overlayPinned = pinned
	a.pinMu.Unlock()
}

// SetOverlayPinned is called by the overlay toolbar's pin button. Overlays are
// always-on-top by default; this keeps the Win32 z-order in sync with the UI.
func (a *App) SetOverlayPinned(pinned bool) {
	a.setOverlayPinned(pinned)
	if hwnd := win.FindWindowByTitle(a.windowTitle); hwnd != 0 {
		win.SetWindowTopmost(hwnd, pinned)
	}
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
	failures := 0
	for {
		select {
		case <-ticker.C:
			health, err := a.client.Health(ctx)
			a.mu.Lock()
			if err != nil {
				failures++
				if failures >= 6 {
					a.startupStatus = "本地引擎未就绪，可在“设置 → 模型”中检查下载源或手动下载模型"
				} else {
					a.startupStatus = "正在连接本地服务..."
				}
			} else {
				failures = 0
				if health.TranslatorStatus != "" {
					a.startupStatus = health.TranslatorStatus
				} else if !health.TranslatorReady {
					a.startupStatus = "正在加载翻译模型..."
				}
				if strings.Contains(health.TranslatorStatus, "失败") ||
					strings.Contains(strings.ToLower(health.TranslatorStatus), "error") {
					a.startupError = health.TranslatorStatus
				}
			}
			ready := err == nil && health.TranslatorReady && time.Since(a.startupStarted) >= 1200*time.Millisecond
			// Background loading never hides or replaces the workbench. On success
			// the banner clears; on failure the timeout branch below explains how
			// to change the download source.
			if ready {
				a.startupLoading = false
				a.startupStatus = "模型加载完成"
				a.startupError = ""
			}
			a.mu.Unlock()
			a.emitState()
			if ready {
				// Keep the workbench visible. Background loading is done; the user
				// may continue translating or adjusting settings.
				return
			}
		case <-deadline.C:
			a.mu.Lock()
			a.startupLoading = false
			a.startupStatus = "模型加载超时"
			a.startupError = "模型加载超时：请在“设置 → 模型”中检查下载源、网络或手动下载模型。"
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

func (a *App) positionMainWindow(ctx context.Context) {
	runtime.WindowSetAlwaysOnTop(ctx, false)
	a.mu.Lock()
	first := !a.mainSized
	a.mainSized = true
	a.mu.Unlock()
	if first {
		width, height := mainWindowSize(ctx)
		runtime.WindowSetSize(ctx, width, height)
	}
	runtime.WindowCenter(ctx)
}

// mainWindowSize picks a comfortable initial workbench size. The Wails window
// is resizable, so this only applies on the first show; user resizes persist.
func mainWindowSize(ctx context.Context) (int, int) {
	width, height := 520, 720
	if screens, err := runtime.ScreenGetAll(ctx); err == nil {
		for _, screen := range screens {
			if screen.IsPrimary || screen.IsCurrent {
				width = clampInt(int(float64(screen.Size.Width)*0.46), 460, 560)
				height = clampInt(int(float64(screen.Size.Height)*0.80), 600, 760)
				break
			}
		}
	}
	return width, height
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
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
		// Keep the first paint fast even when the engine is still booting; the
		// frontend polls again and the header/banner update in place.
		healthCtx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
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
	if err := config.Save(a.baseDir, cfg); err != nil {
		return err
	}
	a.notifyConfigSaved()
	return nil
}

// ModelSearchItem is one row in the "download a model" search results.
type ModelSearchItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Downloads   int64  `json:"downloads"`
	Description string `json:"description"`
	SizeBytes   int64  `json:"size_bytes"`
	Parameters  string `json:"parameters"`
}

// ModelFileItem is one GGUF file inside a searched repository.
type ModelFileItem struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// ModelStatus tells the settings page whether a configured model is on disk.
type ModelStatus struct {
	ID        string `json:"id"`
	Installed bool   `json:"installed"`
	Size      int64  `json:"size"`
	Path      string `json:"path"`
}

// ModelDownloadEvent is streamed through "ui:model-download".
type ModelDownloadEvent struct {
	Status     string  `json:"status"`
	Source     string  `json:"source"`
	Repo       string  `json:"repo"`
	File       string  `json:"file"`
	Name       string  `json:"name"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Percent    float64 `json:"percent"`
	Speed      int64   `json:"speed"`
	Error      string  `json:"error,omitempty"`
}

// SearchModels queries ModelScope/HuggingFace for GGUF repositories.
func (a *App) SearchModels(source, query string) ([]ModelSearchItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	results, err := modelhub.Search(ctx, source, query)
	if err != nil {
		return nil, err
	}
	items := make([]ModelSearchItem, 0, len(results))
	for _, result := range results {
		items = append(items, ModelSearchItem{
			ID:          result.ID,
			Name:        result.Name,
			Source:      result.Source,
			Downloads:   result.Downloads,
			Description: result.Description,
			SizeBytes:   result.SizeBytes,
			Parameters:  result.Parameters,
		})
	}
	return items, nil
}

// ListModelFiles lists the GGUF files inside a repository.
func (a *App) ListModelFiles(source, repo string) ([]ModelFileItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	files, err := modelhub.Files(ctx, source, repo)
	if err != nil {
		return nil, err
	}
	items := make([]ModelFileItem, 0, len(files))
	for _, file := range files {
		items = append(items, ModelFileItem{Name: file.Name, Size: file.Size})
	}
	return items, nil
}

// ModelStatuses reports which of the given models already exist on disk.
func (a *App) ModelStatuses(models []config.ModelEntry) []ModelStatus {
	normalized := config.NormalizeModels(models)
	statuses := make([]ModelStatus, 0, len(normalized))
	for _, entry := range normalized {
		path := config.ModelPath(a.baseDir, entry)
		status := ModelStatus{ID: entry.ID, Path: path}
		if info, err := os.Stat(path); err == nil {
			status.Installed = true
			status.Size = info.Size()
		}
		statuses = append(statuses, status)
	}
	return statuses
}

// StartModelDownload downloads a repository file in the background and streams
// progress through "ui:model-download".
func (a *App) StartModelDownload(source, repo, file, name string) error {
	repo = config.NormalizeModelRepo(repo)
	file = config.NormalizeModelFile(file)
	source = config.NormalizeModelSource(source)
	if repo == "" || file == "" {
		return fmt.Errorf("无效的模型仓库或文件名")
	}
	entry := config.ModelEntry{
		Source: source,
		Repo:   repo,
		File:   file,
		Name:   strings.TrimSpace(name),
		Dir:    config.NormalizeModelDir("", repo),
	}
	dest := config.ModelPath(a.baseDir, entry)
	if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
		return fmt.Errorf("模型文件已存在")
	}

	a.downloadMu.Lock()
	if a.downloading {
		a.downloadMu.Unlock()
		return fmt.Errorf("已有模型正在下载")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.downloadCancel = cancel
	a.downloading = true
	a.downloadMu.Unlock()

	displayName := entry.Name
	if displayName == "" {
		displayName = repo
	}
	log.Printf("model download start: source=%s repo=%s file=%s dest=%s", source, repo, file, dest)
	a.emitModelDownload(ModelDownloadEvent{
		Status: "downloading",
		Source: source,
		Repo:   repo,
		File:   file,
		Name:   displayName,
	})
	go func() {
		defer func() {
			a.downloadMu.Lock()
			a.downloading = false
			a.downloadCancel = nil
			a.downloadMu.Unlock()
		}()
		var lastBytes int64
		lastAt := time.Now()
		lastEmit := time.Now()
		err := modelhub.Download(ctx, source, repo, file, dest, func(progress modelhub.Progress) {
			now := time.Now()
			speed := int64(0)
			if elapsed := now.Sub(lastAt).Seconds(); elapsed > 0.2 {
				speed = int64(float64(progress.Downloaded-lastBytes) / elapsed)
				lastBytes = progress.Downloaded
				lastAt = now
			}
			if now.Sub(lastEmit) < 150*time.Millisecond {
				return
			}
			lastEmit = now
			percent := 0.0
			if progress.Total > 0 {
				percent = float64(progress.Downloaded) / float64(progress.Total) * 100
			}
			a.emitModelDownload(ModelDownloadEvent{
				Status:     "downloading",
				Source:     source,
				Repo:       repo,
				File:       file,
				Name:       displayName,
				Downloaded: progress.Downloaded,
				Total:      progress.Total,
				Percent:    percent,
				Speed:      speed,
			})
		})
		if err != nil {
			status := "error"
			message := err.Error()
			if ctx.Err() != nil {
				status = "canceled"
				message = "下载已取消"
			}
			log.Printf("model download failed: source=%s repo=%s file=%s err=%v", source, repo, file, err)
			a.emitModelDownload(ModelDownloadEvent{
				Status: status, Source: source, Repo: repo, File: file, Name: displayName, Error: message,
			})
			return
		}
		if info, statErr := os.Stat(dest); statErr == nil {
			a.emitModelDownload(ModelDownloadEvent{
				Status:     "done",
				Source:     source,
				Repo:       repo,
				File:       file,
				Name:       displayName,
				Downloaded: info.Size(),
				Total:      info.Size(),
				Percent:    100,
			})
		}
	}()
	return nil
}

// CancelModelDownload stops the running download, keeping the .part file for a
// later resume.
func (a *App) CancelModelDownload() {
	a.downloadMu.Lock()
	cancel := a.downloadCancel
	a.downloadMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// DeleteModelFile removes a downloaded GGUF (and its partial file).
func (a *App) DeleteModelFile(repo, file string) error {
	repo = config.NormalizeModelRepo(repo)
	file = config.NormalizeModelFile(file)
	if repo == "" || file == "" {
		return fmt.Errorf("无效的模型文件")
	}
	dir := config.NormalizeModelDir("", repo)
	dest := filepath.Join(a.baseDir, "models", dir, config.LocalModelFile(file))
	modelsRoot := filepath.Join(a.baseDir, "models") + string(os.PathSeparator)
	if !strings.HasPrefix(dest, modelsRoot) {
		return fmt.Errorf("拒绝删除模型目录之外的文件")
	}
	_ = os.Remove(dest + ".part")
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (a *App) emitModelDownload(payload ModelDownloadEvent) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, eventModelDownload, payload)
}

func (a *App) Translate(text string) (TranslateResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return TranslateResult{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	sourceLang, targetLang := a.languagePair()
	startedAt := time.Now()
	result, err := a.client.TranslateWithLanguages(ctx, text, sourceLang, targetLang)
	latency := time.Since(startedAt).Milliseconds()
	if err != nil {
		return TranslateResult{}, err
	}
	return TranslateResult{
		Source:    text,
		Result:    strings.TrimSpace(result),
		LatencyMs: latency,
	}, nil
}

// languagePair reads the configured source/target languages. Auto is the
// fallback when settings cannot be loaded.
func (a *App) languagePair() (string, string) {
	cfg, err := config.Load(a.baseDir)
	if err != nil {
		return "auto", "auto"
	}
	return cfg.SourceLanguage, cfg.TargetLanguage
}

// OCRTranslateImage accepts a data URL or bare base64 PNG from the workbench
// clipboard/file import and returns the recognized text plus its translation.
func (a *App) OCRTranslateImage(imageData string) (TranslateResult, error) {
	raw := strings.TrimSpace(imageData)
	if strings.HasPrefix(raw, "data:") {
		if comma := strings.Index(raw, ","); comma >= 0 {
			raw = raw[comma+1:]
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return TranslateResult{}, fmt.Errorf("图片数据无效: %w", err)
	}
	if len(decoded) == 0 {
		return TranslateResult{}, fmt.Errorf("图片数据为空")
	}
	const maxImageBytes = 24 << 20
	if len(decoded) > maxImageBytes {
		return TranslateResult{}, fmt.Errorf("图片过大（上限 24MB）")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	sourceLang, targetLang := a.languagePair()
	startedAt := time.Now()
	result, err := a.client.OCRTranslateImageWithLanguages(ctx, decoded, sourceLang, targetLang)
	latency := time.Since(startedAt).Milliseconds()
	if err != nil {
		return TranslateResult{}, err
	}
	source := strings.TrimSpace(result.SourceText)
	if source == "" {
		return TranslateResult{}, fmt.Errorf("未识别到文字")
	}
	return TranslateResult{
		Source:    source,
		Result:    strings.TrimSpace(result.Translation),
		LatencyMs: latency,
	}, nil
}

// HideWindow is called by the frontend close button in daemon mode.
func (a *App) HideWindow() error {
	return a.Hide()
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

	hwnd := win.FindWindowByTitle(a.windowTitle)
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

// ShowSettings implements uidaemon.Handler.
func (a *App) ShowSettings() error {
	if a.overlayMode {
		return nil
	}
	if a.ctx == nil {
		return context.Canceled
	}

	a.setOverlaySizing(false)
	a.mu.Lock()
	a.mode = "settings"
	a.visible = true
	a.settingsOpen = true
	a.cancelIdleExitLocked()
	a.mu.Unlock()

	a.positionMainWindow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	a.emitState()
	return nil
}

// ShowTranslate implements uidaemon.Handler.
func (a *App) ShowTranslate() error {
	if a.overlayMode {
		return nil
	}
	if a.ctx == nil {
		return context.Canceled
	}

	a.setOverlaySizing(false)
	a.mu.Lock()
	a.mode = "translate"
	a.visible = true
	a.settingsOpen = true
	a.cancelIdleExitLocked()
	a.mu.Unlock()

	a.positionMainWindow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
	a.emitState()
	return nil
}

// ShowResult implements uidaemon.Handler.
func (a *App) ShowResult(payload uidaemon.ResultPayload) error {
	if a.overlayMode {
		return nil
	}
	if a.ctx == nil {
		return context.Canceled
	}

	// The compact result popup is 425x300, below the workbench minimum.
	a.setOverlaySizing(true)
	a.mu.Lock()
	wasVisibleResult := a.visible && a.mode == "result"
	a.mode = "result"
	a.visible = true
	// Result can temporarily replace settings view; keep settingsOpen sticky so
	// host continues suspending hotkeys until settings are explicitly closed.
	a.result = ResultView{
		Source:    payload.Source,
		Result:    payload.Result,
		Error:     payload.Error,
		Loading:   payload.Loading,
		LatencyMs: payload.LatencyMs,
	}
	a.cancelIdleExitLocked()
	a.mu.Unlock()

	// Only place the popup on first show. Subsequent loading/result updates must
	// not chase the cursor or reset size, which causes a visible jump.
	if !wasVisibleResult {
		positionResultWindow(a.ctx)
		runtime.WindowSetSize(a.ctx, 425, 300)
		runtime.WindowUnminimise(a.ctx)
		hwnd := win.FindWindowByTitle(a.windowTitle)
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

// ShowOverlay implements uidaemon.Handler. The window is resized to the
// physical screenshot rectangle so translated blocks line up with the pixels
// they were recognized from.
func (a *App) ShowOverlay(payload uidaemon.OverlayPayload) error {
	if a.ctx == nil {
		return context.Canceled
	}

	windowRect := payload.WindowRect
	if windowRect.Width <= 0 || windowRect.Height <= 0 {
		windowRect = payload.Rect
	}
	a.hitMu.Lock()
	a.overlayLive = false
	a.hitMu.Unlock()
	a.setOverlaySizing(true)
	a.prepareOverlayWindow()
	// The screenshot overlay now displays the captured frame as an opaque base,
	// so the whole window is interactive instead of click-through.
	a.setHitTest(
		win.Rect{
			Left:   int32(payload.Rect.X - windowRect.X),
			Top:    int32(payload.Rect.Y - windowRect.Y),
			Right:  int32(payload.Rect.X - windowRect.X + payload.Rect.Width),
			Bottom: int32(payload.Rect.Y - windowRect.Y + payload.Rect.Height),
		},
		false,
		false,
	)
	if hwnd := win.FindWindowByTitle(a.windowTitle); hwnd != 0 {
		a.setCaptureVisibility(hwnd, true)
		win.SetResizable(hwnd, false)
	}

	a.mu.Lock()
	wasOverlay := a.mode == "overlay" && a.visible
	rectChanged := a.overlayRect != windowRect
	if !wasOverlay || rectChanged {
		a.overlayRect = windowRect
	}
	a.mode = "overlay"
	a.visible = true
	a.settingsOpen = false
	a.cancelIdleExitLocked()
	a.mu.Unlock()

	if !wasOverlay || rectChanged {
		if !wasOverlay {
			// Every new screenshot overlay starts pinned/always-on-top.
			a.setOverlayPinned(true)
		}
		a.applyWindowRect(windowRect)
		runtime.WindowUnminimise(a.ctx)
		runtime.WindowSetAlwaysOnTop(a.ctx, a.isOverlayPinned())
		if hwnd := win.FindWindowByTitle(a.windowTitle); hwnd != 0 {
			win.ForceForegroundWindow(hwnd)
		}
	}
	// The host hides the previous overlay before capturing a new selection.
	// Re-show it even when the rect is unchanged, otherwise a same-size
	// selection would leave the overlay hidden.
	if hwnd := win.FindWindowByTitle(a.windowTitle); hwnd != 0 {
		win.SetWindowTopmost(hwnd, a.isOverlayPinned())
		win.ShowWindow(hwnd, win.SWShowNoActivate)
	}

	runtime.EventsEmit(a.ctx, eventOverlay, payload)
	a.emitState()
	return nil
}

// ShowLive implements uidaemon.Handler. The panel covers exactly the monitored
// region and shows its captured frame. It is always capturable so users can
// screenshot the translation with any screen tool; the host skips a live frame
// while the panel overlaps the monitored region, which prevents OCR feedback.
func (a *App) ShowLive(payload uidaemon.LivePayload) error {
	if a.ctx == nil {
		return context.Canceled
	}

	windowRect := payload.WindowRect
	if windowRect.Width <= 0 || windowRect.Height <= 0 {
		windowRect = payload.Anchor
	}
	a.hitMu.Lock()
	a.overlayLive = true
	a.hitMu.Unlock()
	a.setOverlaySizing(true)
	a.prepareOverlayWindow()
	// The live panel also shows the captured frame, so it is an interactive
	// rectangle; the page's edge strips handle resizing.
	a.setHitTest(
		win.Rect{
			Left:   int32(payload.Anchor.X - windowRect.X),
			Top:    int32(payload.Anchor.Y - windowRect.Y),
			Right:  int32(payload.Anchor.X - windowRect.X + payload.Anchor.Width),
			Bottom: int32(payload.Anchor.Y - windowRect.Y + payload.Anchor.Height),
		},
		false,
		false,
	)
	if hwnd := win.FindWindowByTitle(a.windowTitle); hwnd != 0 {
		// Live and screenshot overlays are always capturable. The host avoids
		// OCR feedback by skipping a frame while this panel overlaps the region.
		a.setCaptureVisibility(hwnd, true)
		// Edge resizing is handled by the page so the shaped window keeps a
		// consistent client area; the native thick frame would be clipped away
		// by the window region.
		win.SetResizable(hwnd, false)
	}

	a.mu.Lock()
	wasLive := a.mode == "live" && a.visible
	panelChanged := a.liveRect != windowRect
	if windowRect.Width > 0 && windowRect.Height > 0 {
		a.liveRect = windowRect
	}
	panel := a.liveRect
	a.mode = "live"
	a.visible = true
	a.settingsOpen = false
	a.cancelIdleExitLocked()
	a.mu.Unlock()

	if !wasLive || panelChanged {
		if !wasLive {
			// Every new live session starts pinned/always-on-top.
			a.setOverlayPinned(true)
		}
		a.applyWindowRect(panel)
		runtime.WindowUnminimise(a.ctx)
		runtime.WindowSetAlwaysOnTop(a.ctx, a.isOverlayPinned())
		if hwnd := win.FindWindowByTitle(a.windowTitle); hwnd != 0 {
			win.ForceForegroundWindow(hwnd)
		}
	}
	// The live loop may hide the panel for a paused refresh; always bring it
	// back for the next payload.
	if hwnd := win.FindWindowByTitle(a.windowTitle); hwnd != 0 {
		win.SetWindowTopmost(hwnd, a.isOverlayPinned())
		win.ShowWindow(hwnd, win.SWShowNoActivate)
	}

	runtime.EventsEmit(a.ctx, eventLive, payload)
	a.emitState()
	return nil
}

// applyWindowRect positions the Wails window with physical pixels. Wails'
// WindowSetPosition interprets coordinates relative to the current monitor,
// which is wrong for a pixel-perfect screenshot overlay; MoveWindow is exact.
func (a *App) applyWindowRect(rect uidaemon.ScreenRect) {
	if rect.Width <= 0 || rect.Height <= 0 {
		return
	}
	hwnd := win.FindWindowByTitle(a.windowTitle)
	if hwnd == 0 {
		// The overlay process may still be creating its window when the host
		// pushes the first payload. Wait briefly so the exact physical MoveWindow
		// path is used instead of the DPI-scaled Wails fallback.
		deadline := time.Now().Add(1500 * time.Millisecond)
		for hwnd == 0 && time.Now().Before(deadline) {
			time.Sleep(25 * time.Millisecond)
			hwnd = win.FindWindowByTitle(a.windowTitle)
		}
	}
	if hwnd != 0 {
		// A live window may have been dragged by the user; don't fight the
		// drag when the computed rect already matches its actual position.
		if current, err := win.GetWindowRect(hwnd); err == nil {
			if absInt32(current.Left-int32(rect.X)) <= 2 &&
				absInt32(current.Top-int32(rect.Y)) <= 2 &&
				absInt32(current.Width()-int32(rect.Width)) <= 2 &&
				absInt32(current.Height()-int32(rect.Height)) <= 2 {
				return
			}
		}
		// Move while still hidden, then show, so the overlay never flashes at
		// the workbench's previous size/position.
		win.MoveWindow(hwnd, int32(rect.X), int32(rect.Y), int32(rect.Width), int32(rect.Height), true)
		win.ShowWindow(hwnd, win.SWShowNoActivate)
		// Screenshot/live overlays are always-on-top by default; the pin button
		// can turn this off until the next selection.
		win.SetWindowTopmost(hwnd, a.isOverlayPinned())
		return
	}
	runtime.WindowSetSize(a.ctx, rect.Width, rect.Height)
	runtime.WindowSetPosition(a.ctx, rect.X, rect.Y)
	runtime.WindowShow(a.ctx)
}

func absInt32(value int32) int32 {
	if value < 0 {
		return -value
	}
	return value
}

// SetHotkeyRecording is called by the settings UI while a hotkey field has
// focus. Registered hotkeys are intercepted by Windows before the WebView can
// see them, so they must be temporarily unregistered to record a new combo.
func (a *App) SetHotkeyRecording(recording bool) error {
	if a.notify == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return a.notify.HotkeyHold(ctx, recording)
}

// OverlayMenuItem is one entry in the overlay's separate owner-drawn popup
// menu. Header/note entries are disabled labels; separators have no label.
type OverlayMenuItem struct {
	ID        uint16 `json:"id"`
	Label     string `json:"label"`
	Shortcut  string `json:"shortcut"`
	Icon      string `json:"icon"`
	Checked   bool   `json:"checked"`
	Disabled  bool   `json:"disabled"`
	Separator bool   `json:"separator"`
	Header    bool   `json:"header"`
	Danger    bool   `json:"danger"`
}

// OverlayTheme returns the persisted theme for the overlay process. The
// overlay polls it so a settings change repaints an already-open HUD.
func (a *App) OverlayTheme() string {
	cfg, err := config.Load(a.baseDir)
	if err != nil {
		return "system"
	}
	switch cfg.Theme {
	case "light", "dark", "system":
		return cfg.Theme
	default:
		return "system"
	}
}

// ShowOverlayMenu opens a separate owner-drawn popup at a client point. The
// popup lives outside the shaped window, so opening it never resizes the
// selection or introduces an in-window scrollbar. Returns the selected command
// ID (0 when dismissed).
func (a *App) ShowOverlayMenu(items []OverlayMenuItem, clientX, clientY int, below bool) uint16 {
	hwnd := win.FindWindowByTitle(a.windowTitle)
	if hwnd == 0 || len(items) == 0 {
		log.Printf("overlay menu ignored: hwnd=%d items=%d", hwnd, len(items))
		return 0
	}
	if len(items) > 48 {
		items = items[:48]
	}
	menuItems := make([]win.MenuItem, 0, len(items))
	for _, item := range items {
		label := strings.TrimSpace(item.Label)
		shortcut := strings.TrimSpace(item.Shortcut)
		if !item.Separator && label == "" {
			continue
		}
		if len([]rune(label)) > 120 {
			label = string([]rune(label)[:120])
		}
		if len([]rune(shortcut)) > 24 {
			shortcut = string([]rune(shortcut)[:24])
		}
		menuItems = append(menuItems, win.MenuItem{
			ID:        item.ID,
			Text:      label,
			Shortcut:  shortcut,
			Icon:      item.Icon,
			Disabled:  item.Disabled,
			Separator: item.Separator,
			Checked:   item.Checked,
			Header:    item.Header,
			Danger:    item.Danger,
		})
	}
	if len(menuItems) == 0 {
		return 0
	}
	point := win.ClientToScreen(hwnd, win.Point{X: int32(clientX), Y: int32(clientY)})
	// Let the click's mouse-up finish before entering TrackPopupMenu's modal
	// loop; otherwise the pending button-up dismisses the menu immediately.
	win.ForceForegroundWindow(hwnd)
	time.Sleep(100 * time.Millisecond)
	win.PostMessage(hwnd, 0, 0, 0)

	request := &overlayMenuRequest{x: point.X, y: point.Y, below: below, items: menuItems}
	overlayMenuMu.Lock()
	overlayMenuReq = request
	log.Printf("overlay menu open: items=%d at %d,%d below=%v", len(menuItems), point.X, point.Y, below)
	win.SendMessage(hwnd, wmShowOverlayMenu, 0, 0)
	overlayMenuReq = nil
	overlayMenuMu.Unlock()
	log.Printf("overlay menu selected: %d", request.selected)
	return request.selected
}

// LiveControl lets the panel pause/resume/refresh/stop the host capture loop.
func (a *App) LiveControl(action string) error {
	if a.notify == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return a.notify.LiveControl(ctx, action)
}

// RefreshOverlay asks the host to re-run OCR and translation for the currently
// displayed screenshot.
func (a *App) RefreshOverlay() error {
	if a.notify == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return a.notify.OverlayRefresh(ctx)
}

// Hide implements uidaemon.Handler.
func (a *App) Hide() error {
	a.stopClickOutsideMonitor()
	a.setOverlaySizing(false)
	a.setHitTest(win.Rect{}, false, false)
	a.hitMu.Lock()
	a.regionSig = ""
	a.hitMu.Unlock()

	if a.ctx == nil {
		return nil
	}

	a.mu.Lock()
	wasSettings := a.settingsOpen || a.mode == "settings" || a.mode == "translate"
	wasOverlay := a.mode == "overlay"
	wasLive := a.mode == "live"
	a.visible = false
	a.settingsOpen = false
	if a.mode == "result" {
		a.result = ResultView{}
	}
	if wasOverlay {
		a.overlayRect = uidaemon.ScreenRect{}
	}
	if wasLive {
		a.liveRect = uidaemon.ScreenRect{}
	}
	a.mode = "settings"
	a.mu.Unlock()

	runtime.WindowHide(a.ctx)
	a.scheduleIdleExit()
	if wasSettings {
		a.notifyHidden()
	}
	if wasOverlay {
		a.notifyOverlayClosed()
	}
	if wasLive {
		a.notifyLiveStop()
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

func (a *App) notifyOverlayClosed() {
	if a.notify == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.notify.OverlayClosed(ctx); err != nil {
		log.Printf("notify overlay closed failed: %v", err)
	}
}

func (a *App) notifyLiveStop() {
	if a.notify == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.notify.LiveControl(ctx, "stop"); err != nil {
		log.Printf("notify live stop failed: %v", err)
	}
}

func (a *App) scheduleIdleExit() {
	if !a.daemonMode || a.overlayMode {
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

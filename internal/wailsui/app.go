package wailsui

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"translate-plugin/internal/backend"
	"translate-plugin/internal/config"
	win "translate-plugin/internal/platform/windows"
)

type App struct {
	ctx        context.Context
	baseDir    string
	client     *backend.Client
	supervisor *backend.Supervisor
	mode       string
	result     ResultView
	resultPath string
}

type UIState struct {
	Config config.Config  `json:"config"`
	Health backend.Health `json:"health"`
	Mode   string         `json:"mode"`
	Result ResultView     `json:"result"`
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
		baseDir:    baseDir,
		client:     client,
		supervisor: backend.NewSupervisor(baseDir, client),
		mode:       "settings",
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--result" && i+1 < len(args) {
			app.mode = "result"
			app.resultPath = args[i+1]
			_ = app.loadResult()
			break
		}
	}
	return app
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	if a.mode == "result" {
		positionResultWindow(ctx)
		return
	}
	go func() {
		startCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = a.supervisor.EnsureRunning(startCtx, func(string) {})
	}()
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

	runtime.WindowSetPosition(ctx, int(x), int(y))
	runtime.WindowSetAlwaysOnTop(ctx, true)
}

func (a *App) Shutdown(ctx context.Context) {
	if a.mode == "result" {
		if a.resultPath != "" {
			_ = os.Remove(a.resultPath)
		}
		return
	}
	a.supervisor.Stop()
}

func (a *App) State() (UIState, error) {
	if a.mode == "result" {
		if err := a.loadResult(); err != nil {
			return UIState{}, err
		}
		cfg, err := config.Load(a.baseDir)
		if err != nil {
			return UIState{}, err
		}
		return UIState{Config: cfg, Mode: a.mode, Result: a.result}, nil
	}

	cfg, err := config.Load(a.baseDir)
	if err != nil {
		return UIState{}, err
	}
	health, _ := a.client.Health(context.Background())
	return UIState{Config: cfg, Health: health, Mode: a.mode, Result: a.result}, nil
}

func (a *App) SaveConfig(cfg config.Config) error {
	if err := config.ValidateServerPort(cfg.ServerPort); err != nil {
		return err
	}
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

func (a *App) loadResult() error {
	if a.resultPath == "" {
		return nil
	}
	data, err := os.ReadFile(a.resultPath)
	if err != nil {
		a.result = ResultView{Error: err.Error()}
		return err
	}
	if err := json.Unmarshal(data, &a.result); err != nil {
		a.result = ResultView{Error: err.Error()}
		return err
	}
	return nil
}

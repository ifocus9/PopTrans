package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailswindows "github.com/wailsapp/wails/v2/pkg/options/windows"

	"translate-plugin/internal/config"
	"translate-plugin/internal/logging"
	win "translate-plugin/internal/platform/windows"
	"translate-plugin/internal/wailsui"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	win.EnableDPIAwareness()

	baseDir := config.ResolveBaseDir()
	configureWailsLogging(baseDir)

	launch := wailsui.LaunchOptionsFromEnv()
	launch.ApplyModeArgs(os.Args[1:])
	daemonMode := launch.Daemon
	background := &options.RGBA{A: 0}

	cfg, err := config.Load(baseDir)
	if err != nil {
		cfg = config.Default
	}
	winTheme := wailswindows.SystemDefault
	switch cfg.Theme {
	case "light":
		winTheme = wailswindows.Light
	case "dark":
		winTheme = wailswindows.Dark
	default:
		winTheme = wailswindows.SystemDefault
	}

	windowsOptions := &wailswindows.Options{
		Theme:                winTheme,
		WebviewIsTransparent: true,
		WindowIsTranslucent:  true,
		BackdropType:         wailswindows.Acrylic,
	}
	app := wailsui.NewApp(baseDir, launch)

	err = wails.Run(&options.App{
		Title:         "选中翻译",
		Width:         wailsui.MainDefaultWidth,
		Height:        wailsui.MainDefaultHeight,
		StartHidden:   true,
		DisableResize: false,
		Frameless:     true,
		AlwaysOnTop:   false,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: background,
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		Windows:          windowsOptions,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatalf("wails run: %v", err)
	}
	if daemonMode {
		log.Printf("translate-ui daemon exited")
	}
}

func configureWailsLogging(baseDir string) {
	log.SetPrefix("[translate-wails] ")
	cfg, err := config.Load(baseDir)
	if err != nil {
		_ = logging.Configure("", false)
		return
	}

	logPath := filepath.Join(baseDir, "translate.log")
	if err := logging.Configure(logPath, cfg.LoggingEnabled); err != nil {
		return
	}
	if cfg.LoggingEnabled {
		log.Printf("translate-wails starting, baseDir=%s", baseDir)
	}
}

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

	width, height := windowSize(os.Args)
	resultMode := isResultMode(os.Args)
	background := &options.RGBA{A: 0}
	windowsOptions := &wailswindows.Options{
		Theme:                wailswindows.Dark,
		WebviewIsTransparent: true,
		WindowIsTranslucent:  true,
		BackdropType:         wailswindows.Acrylic,
	}
	app := wailsui.NewApp(baseDir, os.Args)

	err := wails.Run(&options.App{
		Title:         "选中翻译",
		Width:         width,
		Height:        height,
		StartHidden:   !resultMode,
		DisableResize: true,
		Frameless:     true,
		AlwaysOnTop:   resultMode,
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
}

func windowSize(args []string) (int, int) {
	for _, arg := range args {
		if arg == "--result" {
			return 425, 300
		}
	}
	return 458, 640
}

func isResultMode(args []string) bool {
	for _, arg := range args {
		if arg == "--result" {
			return true
		}
	}
	return false
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

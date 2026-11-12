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

	daemonMode := hasArg(os.Args, "--daemon")
	overlayMode := hasArg(os.Args, "--overlay")
	windowTitle := "选中翻译"
	if overlayMode {
		windowTitle = "选中翻译浮层"
	}
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
	if overlayMode {
		// Overlay/live windows get their shape from a window region computed by
		// the page, so everything outside the toolbar and chips is see-through
		// and click-through without acrylic blur or layered-color-key limits.
		background = &options.RGBA{R: 0, G: 0, B: 0, A: 255}
		windowsOptions = &wailswindows.Options{
			Theme:                winTheme,
			WebviewIsTransparent: true,
			WindowIsTranslucent:  false,
			BackdropType:         wailswindows.None,
		}
	}
	app := wailsui.NewApp(baseDir, os.Args)

	err = wails.Run(&options.App{
		Title:         windowTitle,
		Width:         458,
		Height:        640,
		StartHidden:   true,
		DisableResize: true,
		Frameless:     true,
		AlwaysOnTop:   overlayMode,
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

func hasArg(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
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

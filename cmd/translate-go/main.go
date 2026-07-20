package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"translate-plugin/internal/app"
	"translate-plugin/internal/config"
	"translate-plugin/internal/logging"
	win "translate-plugin/internal/platform/windows"
)

func main() {
	win.EnableDPIAwareness()
	runtime.LockOSThread()

	baseDir := config.ResolveBaseDir()
	loggingEnabled := configureLogging(baseDir)
	logHint := ""
	if loggingEnabled {
		logHint = "\n\n详见 translate.log"
	}

	desktopApp, err := app.New()
	if err != nil {
		win.MessageBox(0, "创建 Go 前端失败:\n\n"+err.Error()+logHint, "选中翻译")
		log.Fatalf("create app: %v", err)
	}

	if err := desktopApp.Run(); err != nil {
		win.MessageBox(0, "启动 Go 前端失败:\n\n"+err.Error()+logHint, "选中翻译")
		log.Fatalf("run app: %v", err)
	}
}

func configureLogging(baseDir string) bool {
	log.SetPrefix("[translate-go] ")
	removeLegacyLogs(baseDir)

	cfg, err := config.Load(baseDir)
	if err != nil {
		_ = logging.Configure("", false)
		return false
	}

	logPath := filepath.Join(baseDir, "translate.log")
	if err := logging.Configure(logPath, cfg.LoggingEnabled); err != nil {
		return false
	}
	if cfg.LoggingEnabled {
		log.Printf("translate-go starting, baseDir=%s", baseDir)
		log.Printf("translate-go log path=%s", logPath)
	}
	return cfg.LoggingEnabled
}

func removeLegacyLogs(baseDir string) {
	for _, name := range []string{
		"translate-go.log",
		"translate-wails.log",
		"translate-go-backend.log",
		"ai-engine.log",
	} {
		_ = os.Remove(filepath.Join(baseDir, name))
	}
}

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultServerPort = 8989
	MinServerPort     = 1024
	MaxServerPort     = 65535
)

type Config struct {
	Hotkey           string `json:"hotkey"`
	HotkeyDisplay    string `json:"hotkey_display"`
	OcrEnabled       bool   `json:"ocr_enabled"`
	OcrHotkey        string `json:"ocr_hotkey"`
	OcrHotkeyDisplay string `json:"ocr_hotkey_display"`
	LoggingEnabled   bool   `json:"logging_enabled"`
	ServerPort       int    `json:"server_port"`
	Theme            string `json:"theme"`
}

var Default = Config{
	Hotkey:           "<ctrl>+<alt>+q",
	HotkeyDisplay:    "Ctrl+Alt+Q",
	OcrEnabled:       false,
	OcrHotkey:        "<ctrl>+<alt>+e",
	OcrHotkeyDisplay: "Ctrl+Alt+E",
	LoggingEnabled:   false,
	ServerPort:       DefaultServerPort,
	Theme:            "system",
}

func ResolveBaseDir() string {
	exePath, exeErr := os.Executable()
	if exeErr == nil {
		exeDir := filepath.Dir(exePath)
		if isPackagedRoot(exeDir) {
			return exeDir
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		if dir := findProjectRoot(cwd); dir != "" {
			return dir
		}
	}

	if exeErr != nil {
		return "."
	}

	exeDir := filepath.Dir(exePath)
	if dir := findProjectRoot(exeDir); dir != "" {
		return dir
	}
	return exeDir
}

func Load(baseDir string) (Config, error) {
	cfg := Default
	path := filepath.Join(baseDir, "settings.json")

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default, err
	}

	mergeLoadedDefaults(&cfg)
	return cfg, nil
}

func Save(baseDir string, cfg Config) error {
	mergeDefaults(&cfg)
	if err := ValidateServerPort(cfg.ServerPort); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(baseDir, "settings.json"), data, 0o644)
}

func BackendURL(port int) string {
	if ValidateServerPort(port) != nil {
		port = DefaultServerPort
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func ValidateServerPort(port int) error {
	if port < MinServerPort || port > MaxServerPort {
		return fmt.Errorf("server port must be between %d and %d", MinServerPort, MaxServerPort)
	}
	return nil
}

func mergeDefaults(cfg *Config) {
	if cfg.Hotkey == "" {
		cfg.Hotkey = Default.Hotkey
	}
	if cfg.HotkeyDisplay == "" {
		cfg.HotkeyDisplay = Default.HotkeyDisplay
	}
	if cfg.OcrHotkey == "" {
		cfg.OcrHotkey = Default.OcrHotkey
	}
	if cfg.OcrHotkeyDisplay == "" {
		cfg.OcrHotkeyDisplay = Default.OcrHotkeyDisplay
	}
	if cfg.ServerPort == 0 {
		cfg.ServerPort = Default.ServerPort
	}
	if cfg.Theme != "light" && cfg.Theme != "dark" && cfg.Theme != "system" {
		cfg.Theme = Default.Theme
	}
}

func mergeLoadedDefaults(cfg *Config) {
	mergeDefaults(cfg)
	if ValidateServerPort(cfg.ServerPort) != nil {
		cfg.ServerPort = Default.ServerPort
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findProjectRoot(start string) string {
	dir := filepath.Clean(start)
	for range 6 {
		if isProjectRoot(dir) {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func isProjectRoot(dir string) bool {
	return isPackagedRoot(dir) ||
		(exists(filepath.Join(dir, "backend", "api_server.py")) &&
			exists(filepath.Join(dir, "backend", "requirements.txt")))
}

func isPackagedRoot(dir string) bool {
	return exists(filepath.Join(dir, "ai_engine.exe")) &&
		exists(filepath.Join(dir, "translate-ui.exe"))
}

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultServerPort      = 8989
	MinServerPort          = 1024
	MaxServerPort          = 65535
	DefaultUIIdleMinutes   = 5
	DefaultAIIdleMinutes   = 15
	MaxCpuThreads          = 256
	ModelSourceModelScope  = "modelscope"
	ModelSourceHuggingFace = "huggingface"
	ModelSourceCustom      = "custom"
	DefaultModelDirName    = "Hy-MT2-1.8B-GGUF"
	DefaultModelFilename   = "Hy-MT2-1.8B-Q4_K_M.gguf"
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
	// AccelerationDevice: "auto" | "gpu" | "cpu"
	AccelerationDevice string `json:"acceleration_device"`
	// CpuThreads limits llama.cpp worker threads; 0 lets backend auto-detect.
	CpuThreads int `json:"cpu_threads"`
	// UIIdleMinutes is how long the UI process stays alive after the window is hidden.
	// 0 means never auto-exit. Missing values fall back to DefaultUIIdleMinutes.
	UIIdleMinutes int `json:"ui_idle_minutes"`
	// AIIdleMinutes is how long the AI backend stays alive after its last request.
	// 0 means never auto-exit. Missing values fall back to DefaultAIIdleMinutes.
	AIIdleMinutes int `json:"ai_idle_minutes"`
	// LaunchAtLogin enables starting the app automatically when the user logs in.
	// The host process writes its own executable path to the HKCU Run key.
	LaunchAtLogin bool `json:"launch_at_login"`
	// ModelDownloadSource selects where the translation model is downloaded from:
	// "modelscope" (default), "huggingface", or "custom".
	ModelDownloadSource string `json:"model_download_source"`
	// ModelDownloadBaseURL specifies the mirror root or direct .gguf URL when
	// ModelDownloadSource is "custom".
	ModelDownloadBaseURL string `json:"model_download_base_url"`
}

var Default = Config{
	Hotkey:               "<ctrl>+<alt>+q",
	HotkeyDisplay:        "Ctrl+Alt+Q",
	OcrEnabled:           false,
	OcrHotkey:            "<ctrl>+<alt>+e",
	OcrHotkeyDisplay:     "Ctrl+Alt+E",
	LoggingEnabled:       false,
	ServerPort:           DefaultServerPort,
	Theme:                "system",
	AccelerationDevice:   "auto",
	CpuThreads:           0,
	UIIdleMinutes:        DefaultUIIdleMinutes,
	AIIdleMinutes:        DefaultAIIdleMinutes,
	LaunchAtLogin:        false,
	ModelDownloadSource:  ModelSourceModelScope,
	ModelDownloadBaseURL: "",
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

	rawKeys := map[string]json.RawMessage{}
	_ = json.Unmarshal(data, &rawKeys)

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default, err
	}

	mergeLoadedDefaults(&cfg, rawKeys)
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
	if cfg.AccelerationDevice != "auto" && cfg.AccelerationDevice != "gpu" && cfg.AccelerationDevice != "cpu" {
		cfg.AccelerationDevice = Default.AccelerationDevice
	}
	if cfg.UIIdleMinutes < 0 {
		cfg.UIIdleMinutes = Default.UIIdleMinutes
	}
	if cfg.AIIdleMinutes < 0 {
		cfg.AIIdleMinutes = Default.AIIdleMinutes
	}
	cfg.CpuThreads = clampCpuThreads(cfg.CpuThreads)
	normalizeDownloadSettings(cfg)
}

func mergeLoadedDefaults(cfg *Config, rawKeys map[string]json.RawMessage) {
	mergeDefaults(cfg)
	if ValidateServerPort(cfg.ServerPort) != nil {
		cfg.ServerPort = Default.ServerPort
	}
	if _, ok := rawKeys["acceleration_device"]; !ok {
		cfg.AccelerationDevice = Default.AccelerationDevice
	}
	if _, ok := rawKeys["ui_idle_minutes"]; !ok {
		cfg.UIIdleMinutes = Default.UIIdleMinutes
	}
	if _, ok := rawKeys["ai_idle_minutes"]; !ok {
		cfg.AIIdleMinutes = Default.AIIdleMinutes
	}
	if cfg.UIIdleMinutes < 0 {
		cfg.UIIdleMinutes = Default.UIIdleMinutes
	}
	if cfg.AIIdleMinutes < 0 {
		cfg.AIIdleMinutes = Default.AIIdleMinutes
	}
	cfg.CpuThreads = clampCpuThreads(cfg.CpuThreads)
	normalizeDownloadSettings(cfg)
}

func clampCpuThreads(threads int) int {
	if threads < 0 {
		return 0
	}
	if threads > MaxCpuThreads {
		return MaxCpuThreads
	}
	return threads
}

func normalizeDownloadSettings(cfg *Config) {
	cfg.ModelDownloadSource = NormalizeModelDownloadSource(cfg.ModelDownloadSource)
	cfg.ModelDownloadBaseURL = NormalizeModelDownloadBaseURL(cfg.ModelDownloadBaseURL)
	if cfg.ModelDownloadSource == ModelSourceCustom && cfg.ModelDownloadBaseURL == "" {
		cfg.ModelDownloadSource = ModelSourceModelScope
	}
}

// NormalizeModelDownloadSource normalizes a model download source identifier.
func NormalizeModelDownloadSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", ModelSourceModelScope:
		return ModelSourceModelScope
	case ModelSourceHuggingFace:
		return ModelSourceHuggingFace
	case ModelSourceCustom:
		return ModelSourceCustom
	default:
		return ModelSourceModelScope
	}
}

// NormalizeModelDownloadBaseURL normalizes an HTTP(S) mirror or direct model URL.
func NormalizeModelDownloadBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	return strings.TrimRight(trimmed, "/")
}

// ModelInstalled reports whether the default GGUF model file exists under baseDir.
func ModelInstalled(baseDir string) bool {
	info, err := os.Stat(filepath.Join(baseDir, "models", DefaultModelDirName, DefaultModelFilename))
	return err == nil && !info.IsDir() && info.Size() > 0
}

// UIIdleTimeout returns how long the UI should stay alive after hide.
// A zero duration means the UI should not auto-exit.
func (c Config) UIIdleTimeout() time.Duration {
	if c.UIIdleMinutes < 0 {
		return time.Duration(DefaultUIIdleMinutes) * time.Minute
	}
	if c.UIIdleMinutes == 0 {
		return 0
	}
	return time.Duration(c.UIIdleMinutes) * time.Minute
}

// AIIdleTimeout returns how long the backend should stay alive after its last request.
// A zero duration means the backend should not auto-exit.
func (c Config) AIIdleTimeout() time.Duration {
	if c.AIIdleMinutes < 0 {
		return time.Duration(DefaultAIIdleMinutes) * time.Minute
	}
	if c.AIIdleMinutes == 0 {
		return 0
	}
	return time.Duration(c.AIIdleMinutes) * time.Minute
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

// EnvironmentWith returns environ with key set to value, replacing any existing
// case-insensitive match for key.
func EnvironmentWith(environ []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if strings.HasPrefix(strings.ToUpper(entry), strings.ToUpper(prefix)) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, prefix+value)
}

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestThemeDefaultsToSystem(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "system" {
		t.Fatalf("theme = %q, want system", cfg.Theme)
	}
}

func TestThemeSettingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.Theme = "light"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Theme != "light" {
		t.Fatalf("theme = %q, want light", loaded.Theme)
	}
}

func TestInvalidThemeFallsBackToSystem(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.Theme = "unknown"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Theme != "system" {
		t.Fatalf("theme = %q, want system", loaded.Theme)
	}
}

func TestLoggingDefaultsToDisabled(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LoggingEnabled {
		t.Fatal("logging should require explicit opt-in")
	}
}

func TestLoggingSettingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.LoggingEnabled = true
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.LoggingEnabled {
		t.Fatal("logging_enabled was not persisted")
	}
}

func TestServerPortDefaultsForLegacySettings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"hotkey":"<ctrl>+<alt>+q","theme":"system"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ServerPort != DefaultServerPort {
		t.Fatalf("server port = %d, want %d", loaded.ServerPort, DefaultServerPort)
	}
}

func TestServerPortSettingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.ServerPort = 19090
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ServerPort != 19090 {
		t.Fatalf("server port = %d, want 19090", loaded.ServerPort)
	}
	if got := BackendURL(loaded.ServerPort); got != "http://127.0.0.1:19090" {
		t.Fatalf("backend URL = %q", got)
	}
}

func TestInvalidPersistedServerPortFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"server_port":80,"theme":"system"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ServerPort != DefaultServerPort {
		t.Fatalf("server port = %d, want %d", loaded.ServerPort, DefaultServerPort)
	}
}

func TestSaveRejectsInvalidServerPort(t *testing.T) {
	cfg := Default
	cfg.ServerPort = 70000
	err := Save(t.TempDir(), cfg)
	if err == nil || !strings.Contains(err.Error(), "1024") {
		t.Fatalf("expected port validation error, got %v", err)
	}
}

func TestUIIdleMinutesDefault(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UIIdleMinutes != DefaultUIIdleMinutes {
		t.Fatalf("ui idle minutes = %d, want %d", cfg.UIIdleMinutes, DefaultUIIdleMinutes)
	}
	if cfg.UIIdleTimeout() != time.Duration(DefaultUIIdleMinutes)*time.Minute {
		t.Fatalf("timeout = %s", cfg.UIIdleTimeout())
	}
}

func TestUIIdleMinutesZeroMeansNever(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"ui_idle_minutes":0}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UIIdleMinutes != 0 {
		t.Fatalf("ui idle minutes = %d, want 0", loaded.UIIdleMinutes)
	}
	if loaded.UIIdleTimeout() != 0 {
		t.Fatalf("timeout = %s, want 0", loaded.UIIdleTimeout())
	}
}

func TestUIIdleMinutesMissingUsesDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"theme":"system"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UIIdleMinutes != DefaultUIIdleMinutes {
		t.Fatalf("ui idle minutes = %d, want %d", loaded.UIIdleMinutes, DefaultUIIdleMinutes)
	}
}

func TestAIIdleMinutesDefault(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AIIdleMinutes != DefaultAIIdleMinutes {
		t.Fatalf("ai idle minutes = %d, want %d", cfg.AIIdleMinutes, DefaultAIIdleMinutes)
	}
	if cfg.AIIdleTimeout() != time.Duration(DefaultAIIdleMinutes)*time.Minute {
		t.Fatalf("timeout = %s", cfg.AIIdleTimeout())
	}
}

func TestAIIdleMinutesZeroMeansNever(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"ai_idle_minutes":0}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AIIdleMinutes != 0 {
		t.Fatalf("ai idle minutes = %d, want 0", loaded.AIIdleMinutes)
	}
	if loaded.AIIdleTimeout() != 0 {
		t.Fatalf("timeout = %s, want 0", loaded.AIIdleTimeout())
	}
}

func TestAIIdleMinutesMissingUsesDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"theme":"system"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AIIdleMinutes != DefaultAIIdleMinutes {
		t.Fatalf("ai idle minutes = %d, want %d", loaded.AIIdleMinutes, DefaultAIIdleMinutes)
	}
}

func TestAccelerationDeviceDefaultsToAuto(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccelerationDevice != "auto" {
		t.Fatalf("acceleration device = %q, want auto", cfg.AccelerationDevice)
	}
}

func TestAccelerationDeviceRoundTrip(t *testing.T) {
	for _, dev := range []string{"auto", "gpu", "cpu"} {
		dir := t.TempDir()
		cfg := Default
		cfg.AccelerationDevice = dev
		if err := Save(dir, cfg); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.AccelerationDevice != dev {
			t.Fatalf("acceleration device = %q, want %q", loaded.AccelerationDevice, dev)
		}
	}
}

func TestInvalidAccelerationDeviceFallsBackToAuto(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.AccelerationDevice = "invalid_gpu"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccelerationDevice != "auto" {
		t.Fatalf("acceleration device = %q, want auto", loaded.AccelerationDevice)
	}
}

func TestLaunchAtLoginDefaultsToDisabled(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LaunchAtLogin {
		t.Fatal("launch_at_login should default to false")
	}
}

func TestLaunchAtLoginRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.LaunchAtLogin = true
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.LaunchAtLogin {
		t.Fatal("launch_at_login was not persisted")
	}
}

func TestLaunchAtLoginMissingUsesDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"theme":"system"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LaunchAtLogin {
		t.Fatal("legacy settings should not enable launch_at_login")
	}
}

func TestModelDownloadSourceDefaultsToModelScope(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ModelDownloadSource != ModelSourceModelScope {
		t.Fatalf("expected %q, got %q", ModelSourceModelScope, cfg.ModelDownloadSource)
	}
	if cfg.ModelDownloadBaseURL != "" {
		t.Fatalf("expected empty base URL, got %q", cfg.ModelDownloadBaseURL)
	}
}

func TestModelDownloadSourceRoundTripAndFallback(t *testing.T) {
	dir := t.TempDir()
	cfg := Default
	cfg.ModelDownloadSource = "CUSTOM"
	cfg.ModelDownloadBaseURL = " https://mirror.example.com/hf/ "
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ModelDownloadSource != ModelSourceCustom {
		t.Fatalf("expected %q, got %q", ModelSourceCustom, loaded.ModelDownloadSource)
	}
	if loaded.ModelDownloadBaseURL != "https://mirror.example.com/hf" {
		t.Fatalf("expected trimmed URL, got %q", loaded.ModelDownloadBaseURL)
	}

	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"model_download_source":"custom","model_download_base_url":"ftp://invalid.example.com"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	fallback, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.ModelDownloadSource != ModelSourceModelScope {
		t.Fatalf("expected fallback %q, got %q", ModelSourceModelScope, fallback.ModelDownloadSource)
	}
	if fallback.ModelDownloadBaseURL != "" {
		t.Fatalf("expected invalid base URL to clear, got %q", fallback.ModelDownloadBaseURL)
	}

	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"model_download_source":"unknown-source"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	unknown, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.ModelDownloadSource != ModelSourceModelScope {
		t.Fatalf("expected fallback %q for unknown source, got %q", ModelSourceModelScope, unknown.ModelDownloadSource)
	}
}

func TestModelInstalled(t *testing.T) {
	dir := t.TempDir()
	if ModelInstalled(dir) {
		t.Fatal("expected false before model file exists")
	}
	modelDir := filepath.Join(dir, "models", DefaultModelDirName)
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(modelDir, DefaultModelFilename)
	if err := os.WriteFile(modelPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if ModelInstalled(dir) {
		t.Fatal("expected false for empty placeholder model file")
	}
	if err := os.WriteFile(modelPath, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !ModelInstalled(dir) {
		t.Fatal("expected true when non-empty model file exists")
	}
}

func TestCpuThreadsAndResidentIdleSettings(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CpuThreads != 0 {
		t.Fatalf("cpu_threads should default to 0, got %d", cfg.CpuThreads)
	}

	cfg.CpuThreads = 6
	cfg.UIIdleMinutes = 0
	cfg.AIIdleMinutes = 0
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CpuThreads != 6 || loaded.UIIdleMinutes != 0 || loaded.AIIdleMinutes != 0 {
		t.Fatalf("unexpected persisted config: %+v", loaded)
	}
	if loaded.UIIdleTimeout() != 0 || loaded.AIIdleTimeout() != 0 {
		t.Fatalf("expected 0 resident idle timeouts, got ui=%s ai=%s", loaded.UIIdleTimeout(), loaded.AIIdleTimeout())
	}

	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"cpu_threads":-5}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	neg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if neg.CpuThreads != 0 {
		t.Fatalf("negative cpu_threads should normalize to 0, got %d", neg.CpuThreads)
	}

	if err := os.WriteFile(
		filepath.Join(dir, "settings.json"),
		[]byte(`{"cpu_threads":9999}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	high, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if high.CpuThreads != MaxCpuThreads {
		t.Fatalf("oversized cpu_threads should clamp to %d, got %d", MaxCpuThreads, high.CpuThreads)
	}
}


package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

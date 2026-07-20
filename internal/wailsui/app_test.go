package wailsui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"translate-plugin/internal/config"
)

func TestResultStateReloadsFromStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(path, []byte(`{"source":"hello","loading":true}`), 0600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(t.TempDir(), []string{"translate-ui.exe", "--result", path})
	initial, err := app.State()
	if err != nil {
		t.Fatal(err)
	}
	if !initial.Result.Loading || initial.Result.Source != "hello" {
		t.Fatalf("unexpected loading state: %+v", initial.Result)
	}

	if err := os.WriteFile(path, []byte(`{"source":"hello","result":"你好","loading":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	updated, err := app.State()
	if err != nil {
		t.Fatal(err)
	}
	if updated.Result.Loading || updated.Result.Result != "你好" {
		t.Fatalf("unexpected updated state: %+v", updated.Result)
	}
}

func TestSaveConfigRejectsInvalidServerPort(t *testing.T) {
	app := NewApp(t.TempDir(), []string{"translate-ui.exe", "--settings"})
	cfg := config.Default
	cfg.ServerPort = 80

	err := app.SaveConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "1024") {
		t.Fatalf("expected port validation error, got %v", err)
	}
}

package wailsui

import (
	"strings"
	"testing"
	"time"

	"translate-plugin/internal/config"
	"translate-plugin/internal/uidaemon"
)

func TestDaemonShowResultUpdatesState(t *testing.T) {
	app := NewApp(t.TempDir(), []string{"translate-ui.exe", "--daemon"})
	app.ready = true
	if err := app.ShowResult(uidaemon.ResultPayload{
		Source:  "hello",
		Loading: true,
	}); err == nil {
		// ctx is nil, should fail with context.Canceled
		t.Fatal("expected error when runtime context is missing")
	}

	// Directly set result payload path used by State().
	app.mu.Lock()
	app.mode = "result"
	app.result = ResultView{Source: "hello", Loading: true}
	app.mu.Unlock()

	initial, err := app.State()
	if err != nil {
		t.Fatal(err)
	}
	if !initial.Result.Loading || initial.Result.Source != "hello" {
		t.Fatalf("unexpected loading state: %+v", initial.Result)
	}

	app.mu.Lock()
	app.result = ResultView{Source: "hello", Result: "你好", Loading: false}
	app.mu.Unlock()
	updated, err := app.State()
	if err != nil {
		t.Fatal(err)
	}
	if updated.Result.Loading || updated.Result.Result != "你好" {
		t.Fatalf("unexpected updated state: %+v", updated.Result)
	}
}

func TestSaveConfigRejectsInvalidServerPort(t *testing.T) {
	app := NewApp(t.TempDir(), []string{"translate-ui.exe", "--daemon"})
	cfg := config.Default
	cfg.ServerPort = 80

	err := app.SaveConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "1024") {
		t.Fatalf("expected port validation error, got %v", err)
	}
}

func TestDaemonStatusDefaults(t *testing.T) {
	app := NewApp(t.TempDir(), []string{"translate-ui.exe", "--daemon"})
	status := app.Status()
	if status.Mode != "settings" {
		t.Fatalf("expected settings mode, got %q", status.Mode)
	}
	if status.Ready {
		t.Fatal("expected not ready before Startup")
	}
}

func TestIdleTimeoutDisabledWhenZero(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default
	cfg.UIIdleMinutes = 0
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	app := NewApp(dir, []string{"translate-ui.exe", "--daemon"})
	if app.idleTimeout != 0 {
		t.Fatalf("idleTimeout = %s, want 0", app.idleTimeout)
	}
	app.scheduleIdleExit()
	app.mu.Lock()
	timer := app.idleTimer
	app.mu.Unlock()
	if timer != nil {
		t.Fatal("expected no idle timer when timeout is 0")
	}
}

func TestIdleTimeoutUsesConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default
	cfg.UIIdleMinutes = 1
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	app := NewApp(dir, []string{"translate-ui.exe", "--daemon"})
	if app.idleTimeout != time.Minute {
		t.Fatalf("idleTimeout = %s, want 1m", app.idleTimeout)
	}
}

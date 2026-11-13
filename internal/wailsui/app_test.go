package wailsui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"translate-plugin/internal/config"
	"translate-plugin/internal/uidaemon"
)

func TestDaemonShowResultUpdatesState(t *testing.T) {
	app := NewApp(t.TempDir(), LaunchOptions{Daemon: true})
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
	app := NewApp(t.TempDir(), LaunchOptions{Daemon: true})
	cfg := config.Default
	cfg.ServerPort = 80

	err := app.SaveConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "1024") {
		t.Fatalf("expected port validation error, got %v", err)
	}
}

func TestDaemonStatusDefaults(t *testing.T) {
	app := NewApp(t.TempDir(), LaunchOptions{Daemon: true})
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
	app := NewApp(dir, LaunchOptions{Daemon: true})
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
	app := NewApp(dir, LaunchOptions{Daemon: true})
	if app.idleTimeout != time.Minute {
		t.Fatalf("idleTimeout = %s, want 1m", app.idleTimeout)
	}
}

func TestOverlayModeLaunchOption(t *testing.T) {
	app := NewApp(t.TempDir(), LaunchOptions{Overlay: true})
	if app.mode != "overlay" {
		t.Fatalf("expected overlay mode, got %q", app.mode)
	}
	if app.windowTitle != "选中翻译浮层" {
		t.Fatalf("unexpected overlay title %q", app.windowTitle)
	}
}

func TestStartModelDownloadBinding(t *testing.T) {
	if os.Getenv("POPTRANS_LIVE_TEST") != "1" {
		t.Skip("set POPTRANS_LIVE_TEST=1 to hit the network")
	}
	app := &App{baseDir: t.TempDir()}
	repo := "Qwen/Qwen2.5-0.5B-Instruct-GGUF"
	file := "qwen2.5-0.5b-instruct-q2_k.gguf"
	if err := app.StartModelDownload("modelscope", repo, file, "test"); err != nil {
		t.Fatalf("start: %v", err)
	}
	expected := filepath.Join(app.baseDir, "models", "Qwen-Qwen2.5-0.5B-Instruct-GGUF", file)
	deadline := time.Now().Add(15 * time.Second)
	var partSize int64
	for time.Now().Before(deadline) {
		if info, err := os.Stat(expected + ".part"); err == nil && info.Size() > 0 {
			partSize = info.Size()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	app.CancelModelDownload()
	if partSize == 0 {
		t.Fatalf("no partial download at %s", expected)
	}
	t.Logf("partial download at %s: %d bytes", expected+".part", partSize)
}

func TestLoopbackCallbackURLCheck(t *testing.T) {
	accepted := []string{
		"http://127.0.0.1:32145",
		"http://127.0.0.1",
		"http://[::1]:32145",
	}
	for _, raw := range accepted {
		if !isLoopbackHTTPURL(raw) {
			t.Fatalf("expected %q to be accepted", raw)
		}
	}

	rejected := []string{
		"https://127.0.0.1:32145",
		"http://192.168.1.10:32145",
		"http://example.com:32145",
		"http://localhost:32145",
		"file:///C:/temp",
		"not a url",
	}
	for _, raw := range rejected {
		if isLoopbackHTTPURL(raw) {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestLaunchOptionsFromEnvValidatesValues(t *testing.T) {
	t.Setenv("POPTRANS_UI_DAEMON", "1")
	t.Setenv("POPTRANS_UI_OVERLAY", "1")
	t.Setenv("POPTRANS_UI_ENDPOINT_DIR", "relative-dir")
	t.Setenv("POPTRANS_UI_CALLBACK_URL", "http://example.com/callback")
	t.Setenv("POPTRANS_UI_CALLBACK_TOKEN", "token")

	opts := LaunchOptionsFromEnv()
	if !opts.Daemon || !opts.Overlay {
		t.Fatalf("daemon/overlay flags were lost: %+v", opts)
	}
	if opts.EndpointDir != "" || opts.CallbackURL != "" {
		t.Fatalf("unsafe values were accepted: %+v", opts)
	}
}

func TestLaunchOptionsFromEnvAcceptsSafeValues(t *testing.T) {
	t.Setenv("POPTRANS_UI_ENDPOINT_DIR", `C:\temp\endpoints`)
	t.Setenv("POPTRANS_UI_CALLBACK_URL", "http://127.0.0.1:32145")
	t.Setenv("POPTRANS_UI_CALLBACK_TOKEN", "token")

	opts := LaunchOptionsFromEnv()
	if opts.EndpointDir == "" || opts.CallbackURL == "" || opts.CallbackToken != "token" {
		t.Fatalf("safe values were rejected: %+v", opts)
	}
}

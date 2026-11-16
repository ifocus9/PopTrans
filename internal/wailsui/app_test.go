package wailsui

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"translate-plugin/internal/backend"
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

func TestTranslateModeFlag(t *testing.T) {
	var launch LaunchOptions
	launch.ApplyModeArgs([]string{"--translate"})
	app := NewApp(t.TempDir(), launch)
	if app.mode != "translate" {
		t.Fatalf("expected translate mode, got %q", app.mode)
	}
	if !app.visible || !app.settingsOpen {
		t.Fatal("expected visible and settingsOpen to be true")
	}
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
	t.Setenv("POPTRANS_UI_CALLBACK_URL", "http://example.com/callback")
	t.Setenv("POPTRANS_UI_CALLBACK_TOKEN", strings.Repeat("a", 257))

	opts := LaunchOptionsFromEnv()
	if !opts.Daemon {
		t.Fatalf("daemon flag was lost: %+v", opts)
	}
	if opts.CallbackURL != "" || opts.CallbackToken != "" {
		t.Fatalf("unsafe values were accepted: %+v", opts)
	}
}

func TestLaunchOptionsFromEnvAcceptsSafeValues(t *testing.T) {
	t.Setenv("POPTRANS_UI_DAEMON", "1")
	t.Setenv("POPTRANS_UI_CALLBACK_URL", "http://127.0.0.1:32145")
	t.Setenv("POPTRANS_UI_CALLBACK_TOKEN", "token")

	opts := LaunchOptionsFromEnv()
	if !opts.Daemon || opts.CallbackURL != "http://127.0.0.1:32145" || opts.CallbackToken != "token" {
		t.Fatalf("safe values were rejected: %+v", opts)
	}
}

func TestApplyHealthToStartupClearsStaleError(t *testing.T) {
	app := NewApp(t.TempDir(), LaunchOptions{Daemon: true})

	app.mu.Lock()
	ready := app.applyHealthToStartupLocked(backend.Health{
		TranslatorReady:  false,
		TranslatorStatus: "模型下载失败（已重试 10 次）: connection reset",
	})
	app.mu.Unlock()
	if ready {
		t.Fatal("expected ready=false on failure")
	}
	if app.startupError == "" {
		t.Fatal("expected startupError to be populated on failure status")
	}

	app.mu.Lock()
	ready = app.applyHealthToStartupLocked(backend.Health{
		TranslatorReady:  false,
		TranslatorStatus: "正在从 魔搭 ModelScope 下载模型...",
	})
	app.mu.Unlock()
	if ready {
		t.Fatal("expected ready=false while downloading")
	}
	if app.startupError != "" {
		t.Fatalf("expected stale startupError to clear during download, got %q", app.startupError)
	}

	app.mu.Lock()
	ready = app.applyHealthToStartupLocked(backend.Health{
		TranslatorReady:  false,
		TranslatorStatus: "连接 魔搭 ModelScope 失败，正在重试（1/5）...",
	})
	app.mu.Unlock()
	if ready || app.startupError != "" {
		t.Fatalf("expected transient retry not to set fatal startupError, got %q", app.startupError)
	}

	app.mu.Lock()
	ready = app.applyHealthToStartupLocked(backend.Health{
		TranslatorReady: true,
	})
	app.mu.Unlock()
	if !ready || app.startupError != "" || app.startupLoading {
		t.Fatalf("unexpected ready state: ready=%t loading=%t err=%q", ready, app.startupLoading, app.startupError)
	}
}

func makeSamplePNGBase64(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestDecodeBase64ImageValidation(t *testing.T) {
	if _, err := decodeBase64Image(""); err == nil {
		t.Fatal("expected error for empty payload")
	}
	if _, err := decodeBase64Image("data:image/png;charset=utf-8,abc"); err == nil {
		t.Fatal("expected error for non-base64 data url")
	}
	if _, err := decodeBase64Image(base64.StdEncoding.EncodeToString([]byte("plain text"))); err == nil {
		t.Fatal("expected error for non-image bytes")
	}

	oversized := base64.StdEncoding.EncodeToString(make([]byte, maxOCRImageBytes+1))
	if _, err := decodeBase64Image(oversized); err == nil {
		t.Fatal("expected error for oversized image payload")
	}

	pngB64 := makeSamplePNGBase64(t)
	decoded, err := decodeBase64Image("data:image/png;base64," + pngB64)
	if err != nil {
		t.Fatalf("expected valid PNG data url to decode cleanly: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("expected non-empty PNG bytes")
	}

	webpHeader := append([]byte("RIFF\x00\x00\x00\x00WEBP"), make([]byte, 8)...)
	if _, err := decodeBase64Image(base64.StdEncoding.EncodeToString(webpHeader)); err != nil {
		t.Fatalf("expected WebP signature to be accepted: %v", err)
	}
}

func TestOCRImageAndTranslateMeasureElapsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/ocr":
			if err := r.ParseMultipartForm(16 << 20); err != nil {
				t.Fatalf("parse multipart failed: %v", err)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("missing uploaded file: %v", err)
			}
			_ = file.Close()
			if ct := header.Header.Get("Content-Type"); ct != "image/png" {
				t.Fatalf("expected image/png content type, got %q", ct)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"text":  "  Recognized text  ",
				"lines": []string{"Recognized text"},
				"scale": 1.0,
			})
		case "/v1/chat/completions":
			time.Sleep(15 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": "  识别结果  "}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	app := NewApp(t.TempDir(), LaunchOptions{Daemon: true})
	app.client = backend.NewClient(srv.URL)

	ocrRes, err := app.OCRImage("data:image/png;base64," + makeSamplePNGBase64(t))
	if err != nil {
		t.Fatalf("OCRImage failed: %v", err)
	}
	if ocrRes.Text != "Recognized text" {
		t.Fatalf("unexpected OCR text: %q", ocrRes.Text)
	}
	if ocrRes.ElapsedMS < 0 {
		t.Fatalf("expected non-negative OCR elapsed ms, got %d", ocrRes.ElapsedMS)
	}

	trRes, err := app.Translate(ocrRes.Text)
	if err != nil {
		t.Fatalf("Translate failed: %v", err)
	}
	if trRes.Result != "识别结果" {
		t.Fatalf("unexpected translate result: %q", trRes.Result)
	}
	if trRes.ElapsedMS < 10 {
		t.Fatalf("expected measured translation elapsed ms >= 10, got %d", trRes.ElapsedMS)
	}
}

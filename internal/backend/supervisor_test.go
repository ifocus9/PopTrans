package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureRunningReturnsTranslatorInitializationErrorImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok","translator_ready":false,"translator_status":"翻译引擎初始化失败: incompatible dependency","ocr_loaded":false}`)
	}))
	defer server.Close()

	supervisor := NewSupervisor(t.TempDir(), NewClient(server.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	started := time.Now()
	err := supervisor.EnsureRunning(ctx, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "incompatible dependency") {
		t.Fatalf("expected initialization error, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("initialization error took too long: %s", elapsed)
	}
}

func TestBackendCommandPrefersPackagedEngine(t *testing.T) {
	baseDir := t.TempDir()
	enginePath := filepath.Join(baseDir, "ai_engine.exe")
	if err := os.WriteFile(enginePath, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "backend", "api_server.py"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	supervisor := NewSupervisor(baseDir, NewClient("http://127.0.0.1:0"))
	cmd, name, err := supervisor.backendCommand()
	if err != nil {
		t.Fatal(err)
	}
	if name != "ai_engine.exe" || cmd.Path != enginePath {
		t.Fatalf("expected packaged engine, got name=%q path=%q", name, cmd.Path)
	}
}

func TestBackendCommandUsesPythonForDevelopment(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "backend", "api_server.py"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	supervisor := NewSupervisor(baseDir, NewClient("http://127.0.0.1:0"))
	cmd, name, err := supervisor.backendCommand()
	if err != nil {
		t.Fatal(err)
	}
	if name != "python -m backend.api_server" || len(cmd.Args) != 3 || cmd.Args[1] != "-m" || cmd.Args[2] != "backend.api_server" {
		t.Fatalf("expected Python development command, got name=%q args=%q", name, cmd.Args)
	}
}

func TestEnvironmentWithReplacesServerPort(t *testing.T) {
	env := environmentWith(
		[]string{"PATH=C:\\tools", "translate_server_port=8989"},
		serverPortEnv,
		"19090",
	)

	var matches []string
	for _, entry := range env {
		if strings.HasPrefix(strings.ToUpper(entry), serverPortEnv+"=") {
			matches = append(matches, entry)
		}
	}
	if len(matches) != 1 || matches[0] != serverPortEnv+"=19090" {
		t.Fatalf("server port environment = %q", matches)
	}
}

func TestTerminateProcessTreeAllowsNilProcess(t *testing.T) {
	if err := terminateProcessTree(nil); err != nil {
		t.Fatal(err)
	}
}

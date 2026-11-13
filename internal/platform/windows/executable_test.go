package windows

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrustedExecutableAcceptsBundledBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ai_engine.exe")
	if err := os.WriteFile(path, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := TrustedExecutable(path, "ai_engine.exe")
	if err != nil {
		t.Fatalf("TrustedExecutable: %v", err)
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
}

func TestTrustedExecutableRejectsUnexpectedName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evil.exe")
	if err := os.WriteFile(path, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := TrustedExecutable(path, "ai_engine.exe"); err == nil {
		t.Fatal("expected a name mismatch error")
	}
}

func TestTrustedExecutableRejectsRelativePath(t *testing.T) {
	if _, err := TrustedExecutable("ai_engine.exe", "ai_engine.exe"); err == nil {
		t.Fatal("expected a relative path error")
	}
}

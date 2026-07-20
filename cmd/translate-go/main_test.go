package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveLegacyLogsKeepsUnifiedLog(t *testing.T) {
	dir := t.TempDir()
	legacy := []string{
		"translate-go.log",
		"translate-wails.log",
		"translate-go-backend.log",
		"ai-engine.log",
	}
	for _, name := range append(legacy, "translate.log") {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	removeLegacyLogs(dir)

	for _, name := range legacy {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("legacy log still exists: %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "translate.log")); err != nil {
		t.Fatalf("unified log was removed: %v", err)
	}
}

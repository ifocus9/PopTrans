package logging

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureDisabledDoesNotCreateLogFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disabled.log")
	if err := Configure(path, false); err != nil {
		t.Fatal(err)
	}
	defer Close()

	log.Print("discarded message")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled logging created %s", path)
	}
}

func TestConfigureEnabledWritesLogFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enabled.log")
	if err := Configure(path, true); err != nil {
		t.Fatal(err)
	}

	log.Print("expected message")
	Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "expected message") {
		t.Fatalf("log file did not contain expected message: %q", data)
	}
}

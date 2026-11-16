package windows

import (
	"path/filepath"
	"testing"
)

func TestKillProcessesByImagePathRejectsEmptyPath(t *testing.T) {
	for _, input := range []string{"", "   ", "."} {
		if _, err := KillProcessesByImagePath(input); err == nil {
			t.Fatalf("expected error for invalid path %q", input)
		}
	}
}

func TestKillProcessesByImagePathReturnsZeroForMissingBinary(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.exe")
	killed, err := KillProcessesByImagePath(missing)
	if err != nil {
		t.Fatalf("KillProcessesByImagePath(%q): %v", missing, err)
	}
	if killed != 0 {
		t.Fatalf("killed = %d, want 0", killed)
	}
}


package runtimebundle

import (
	"os"
	"path/filepath"
)

// DirName is the subdirectory (next to PopTrans.exe) where the bundled UI and
// AI engine are extracted on first launch.
const DirName = "runtime"

// Paths points at the runnable UI and AI engine executables.
type Paths struct {
	UIPath     string
	EnginePath string
}

func runtimeDir(baseDir string) string {
	return filepath.Join(baseDir, DirName)
}

func firstExisting(candidates []string) string {
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

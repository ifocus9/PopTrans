//go:build bundle

package runtimebundle

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The release build copies the compiled UI and AI engine into assets/ and
// builds the host with `-tags bundle`. The result is a single PopTrans.exe.
//
//go:embed assets/translate-ui.exe
var uiExe []byte

//go:embed assets/ai_engine.exe
var engineExe []byte

// buildID is stamped by the build script (-ldflags -X ...buildID=...) so an
// updated PopTrans.exe replaces the extracted workers even when the new binary
// happens to have the same size as the old one.
var buildID = ""

func Ensure(baseDir string) (Paths, error) {
	// Keep the visible distribution folder to a single PopTrans.exe: extract
	// the worker executables into the per-user local app data folder.
	dir := ""
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dir = filepath.Join(local, "PopTrans", DirName)
	} else {
		dir = runtimeDir(baseDir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		// Fall back to the application directory if LocalAppData is unusable.
		dir = runtimeDir(baseDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Paths{}, fmt.Errorf("create runtime dir %s: %w", dir, err)
		}
	}

	marker := filepath.Join(dir, ".build-id")
	force := false
	if buildID != "" {
		current, err := os.ReadFile(marker)
		force = err != nil || strings.TrimSpace(string(current)) != buildID
	}

	uiPath, err := extractIfNeeded(dir, "translate-ui.exe", uiExe, force)
	if err != nil {
		return Paths{}, err
	}
	enginePath, err := extractIfNeeded(dir, "ai_engine.exe", engineExe, force)
	if err != nil {
		return Paths{}, err
	}
	if buildID != "" {
		_ = os.WriteFile(marker, []byte(buildID), 0o644)
	}
	return Paths{UIPath: uiPath, EnginePath: enginePath}, nil
}

func extractIfNeeded(dir, name string, data []byte, force bool) (string, error) {
	target := filepath.Join(dir, name)
	if !force {
		if info, err := os.Stat(target); err == nil && info.Size() == int64(len(data)) {
			return target, nil
		}
	}

	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return "", fmt.Errorf("extract %s: %w", name, err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("install %s: %w", name, err)
	}
	return target, nil
}

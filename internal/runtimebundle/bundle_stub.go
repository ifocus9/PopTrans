//go:build !bundle

package runtimebundle

import "path/filepath"

// Ensure is a no-op for development builds. The normal layout (build/bin,
// dist-go, or the repository root) is discovered by the callers.
func Ensure(baseDir string) (Paths, error) {
	ui := firstExisting([]string{
		filepath.Join(runtimeDir(baseDir), "translate-ui.exe"),
		filepath.Join(baseDir, "build", "bin", "translate-ui.exe"),
		filepath.Join(baseDir, "translate-ui.exe"),
	})
	engine := firstExisting([]string{
		filepath.Join(runtimeDir(baseDir), "ai_engine.exe"),
		filepath.Join(baseDir, "ai_engine.exe"),
	})
	return Paths{UIPath: ui, EnginePath: engine}, nil
}

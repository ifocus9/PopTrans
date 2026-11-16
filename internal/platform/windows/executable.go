package windows

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TrustedExecutable validates that path is an absolute path to a regular file
// with the expected executable name so supervisor and UI daemon launches cannot
// start an arbitrary program via a tampered path.
func TrustedExecutable(path, expectedName string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("executable path is empty")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("executable path is not absolute: %s", path)
	}
	cleaned := filepath.Clean(path)
	if !strings.EqualFold(filepath.Base(cleaned), expectedName) {
		return "", fmt.Errorf("unexpected executable name: %s", filepath.Base(cleaned))
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("executable path is a directory: %s", cleaned)
	}
	return cleaned, nil
}


package windows

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// runKeyPath is the current-user Run key used for login autostart entries.
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// launchCommand builds the Run-key command line. The executable path must be
// quoted so install directories containing spaces parse correctly (and cannot
// be hijacked via unquoted-path prefix matching). The --autostart flag tells
// the host to skip the startup window and run silently in the tray.
func launchCommand(exePath string) string {
	return fmt.Sprintf(`"%s" --autostart`, exePath)
}

// SetLaunchAtLogin enables or disables launching the app for the current user
// when they log in. exePath must be an absolute path to the executable;
// callers should pass the host process path (os.Executable) so the entry stays
// valid even if the install directory is moved. Deleting a missing value is
// treated as a no-op, not an error.
func SetLaunchAtLogin(appName, exePath string, enable bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if !enable {
		err := key.DeleteValue(appName)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	if exePath == "" {
		return errors.New("empty executable path")
	}
	return key.SetStringValue(appName, launchCommand(exePath))
}

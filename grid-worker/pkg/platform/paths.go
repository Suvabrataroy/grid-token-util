package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ConfigDir returns the platform-appropriate configuration directory for grid-worker.
// On Windows: %APPDATA%\grid-worker
// On Linux/macOS: ~/.grid-worker
func ConfigDir() string {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(appData, "grid-worker")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
		if home == "" {
			home = "/tmp"
		}
	}
	return filepath.Join(home, ".grid-worker")
}

// WorkspaceDir returns the directory used for task workspaces.
func WorkspaceDir() string {
	return filepath.Join(ConfigDir(), "workspace")
}

// LogDir returns the directory used for log files.
func LogDir() string {
	return filepath.Join(ConfigDir(), "logs")
}

// SocketPath returns the platform-appropriate IPC socket path.
// On Unix: /tmp/grid-worker.sock
// On Windows: \\.\pipe\grid-worker
func SocketPath() string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\grid-worker`
	}
	return "/tmp/grid-worker.sock"
}

// EnsureDirs creates all required directories with 0700 permissions.
func EnsureDirs() error {
	dirs := []string{
		ConfigDir(),
		WorkspaceDir(),
		LogDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", dir, err)
		}
	}
	return nil
}

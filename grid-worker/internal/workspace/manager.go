package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

// Manager manages per-task workspaces within a base directory.
type Manager struct {
	basePath string
	log      zerolog.Logger
}

// New creates a new workspace Manager.
func New(basePath string, log zerolog.Logger) *Manager {
	return &Manager{
		basePath: basePath,
		log:      log.With().Str("component", "workspace-manager").Logger(),
	}
}

// Create creates a workspace directory for the given task ID and returns its path.
// The directory is created with 0700 permissions.
func (m *Manager) Create(taskID string) (string, error) {
	wsPath := m.taskPath(taskID)

	if err := os.MkdirAll(wsPath, 0700); err != nil {
		return "", fmt.Errorf("create workspace for task %s: %w", taskID, err)
	}

	m.log.Debug().
		Str("task_id", taskID).
		Str("path", wsPath).
		Msg("workspace created")

	return wsPath, nil
}

// Cleanup removes the workspace directory for the given task ID.
func (m *Manager) Cleanup(taskID string) error {
	wsPath := m.taskPath(taskID)

	if _, err := os.Stat(wsPath); os.IsNotExist(err) {
		return nil // already gone
	}

	if err := os.RemoveAll(wsPath); err != nil {
		return fmt.Errorf("cleanup workspace for task %s: %w", taskID, err)
	}

	m.log.Debug().
		Str("task_id", taskID).
		Str("path", wsPath).
		Msg("workspace cleaned up")

	return nil
}

// AssertContained verifies that filePath is within workspacePath, preventing
// path traversal attacks. Returns an error if filePath escapes the workspace.
func (m *Manager) AssertContained(workspacePath, filePath string) error {
	// Resolve both paths to their absolute, cleaned forms
	absWorkspace, err := filepath.Abs(workspacePath)
	if err != nil {
		return fmt.Errorf("resolve workspace path: %w", err)
	}

	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("resolve file path: %w", err)
	}

	// Ensure workspace ends with separator to prevent prefix attacks
	// e.g., /tmp/workspace vs /tmp/workspace-evil
	wsWithSep := absWorkspace
	if !strings.HasSuffix(wsWithSep, string(filepath.Separator)) {
		wsWithSep += string(filepath.Separator)
	}

	if absFile != absWorkspace && !strings.HasPrefix(absFile, wsWithSep) {
		return fmt.Errorf("path traversal detected: %q escapes workspace %q", filePath, workspacePath)
	}

	return nil
}

// DiskUsageGB returns the total disk usage of a task's workspace in gigabytes.
func (m *Manager) DiskUsageGB(taskID string) (float64, error) {
	wsPath := m.taskPath(taskID)

	var totalBytes int64
	err := filepath.WalkDir(wsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip inaccessible files
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			totalBytes += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk workspace for task %s: %w", taskID, err)
	}

	return float64(totalBytes) / (1 << 30), nil
}

// taskPath returns the full path to a task's workspace directory.
func (m *Manager) taskPath(taskID string) string {
	// Sanitize taskID to prevent directory traversal
	safe := filepath.Base(taskID)
	if safe == "." || safe == ".." {
		safe = "_invalid_"
	}
	return filepath.Join(m.basePath, safe)
}

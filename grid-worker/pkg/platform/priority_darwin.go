//go:build darwin

package platform

import (
	"fmt"
	"syscall"
)

// SetLowPriority sets the process priority to 20 (lowest) on macOS.
func SetLowPriority() error {
	if err := syscall.Setpriority(syscall.PRIO_PROCESS, 0, 20); err != nil {
		return fmt.Errorf("setpriority failed: %w", err)
	}
	return nil
}

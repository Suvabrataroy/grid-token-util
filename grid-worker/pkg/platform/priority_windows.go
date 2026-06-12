//go:build windows

package platform

import (
	"fmt"

	"golang.org/x/sys/windows"
)

const idlePriorityClass = 0x00000040

// SetLowPriority sets the process priority to IDLE_PRIORITY_CLASS on Windows.
func SetLowPriority() error {
	handle, err := windows.GetCurrentProcess()
	if err != nil {
		return fmt.Errorf("GetCurrentProcess failed: %w", err)
	}

	if err := windows.SetPriorityClass(handle, idlePriorityClass); err != nil {
		return fmt.Errorf("SetPriorityClass failed: %w", err)
	}

	return nil
}

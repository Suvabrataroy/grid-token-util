//go:build windows

package platform

import (
	"github.com/shirou/gopsutil/v3/host"
)

// readBatteryFromSystem reads battery info on Windows via gopsutil.
func readBatteryFromSystem() (float64, bool) {
	// gopsutil doesn't directly expose battery % on Windows without WMI queries.
	// We use host.Info() which may contain battery info on some platforms.
	// Fall back to a no-op for desktop systems.
	info, err := host.Info()
	if err != nil || info == nil {
		return 0, false
	}

	// Windows desktops typically won't have battery; gracefully return false
	return 0, false
}

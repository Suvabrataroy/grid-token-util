package platform

import (
	"github.com/shirou/gopsutil/v3/host"
)

// BatteryPercent returns the current battery charge percentage and whether a battery was found.
// On desktop systems without a battery it returns (0, false).
func BatteryPercent() (float64, bool) {
	sensors, err := host.SensorsTemperatures()
	if err == nil && len(sensors) > 0 {
		// gopsutil doesn't expose battery directly via host; use platform sensors path
		// This is a fallback — actual battery reading uses the battery sub-package
	}

	// Use a direct approach via gopsutil battery info through host info
	info, err := host.Info()
	if err != nil {
		return 0, false
	}
	_ = info // host.Info() does not expose battery; use sensors approach below

	return readBatteryPercent()
}

// readBatteryPercent reads battery info using available system interfaces.
func readBatteryPercent() (float64, bool) {
	// gopsutil/v3 does not have a standalone battery package.
	// We use the host sensors to infer battery presence.
	// On Linux: read from /sys/class/power_supply
	// On macOS: ioreg
	// On Windows: WMI
	// For portability, we attempt a best-effort read.

	percent, found := readBatteryFromSystem()
	return percent, found
}

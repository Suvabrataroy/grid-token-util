//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// readBatteryFromSystem reads battery percentage from /sys/class/power_supply on Linux.
func readBatteryFromSystem() (float64, bool) {
	const powerSupplyDir = "/sys/class/power_supply"

	entries, err := os.ReadDir(powerSupplyDir)
	if err != nil {
		return 0, false
	}

	for _, entry := range entries {
		name := entry.Name()
		// Battery entries are typically named BAT0, BAT1, etc.
		if !strings.HasPrefix(strings.ToUpper(name), "BAT") {
			continue
		}

		typeFile := filepath.Join(powerSupplyDir, name, "type")
		typeBytes, err := os.ReadFile(typeFile)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(typeBytes)) != "Battery" {
			continue
		}

		capacityFile := filepath.Join(powerSupplyDir, name, "capacity")
		capacityBytes, err := os.ReadFile(capacityFile)
		if err != nil {
			continue
		}

		percent, err := strconv.ParseFloat(strings.TrimSpace(string(capacityBytes)), 64)
		if err != nil {
			continue
		}

		return percent, true
	}

	return 0, false
}

//go:build darwin

package platform

import (
	"os/exec"
	"strconv"
	"strings"
)

// readBatteryFromSystem reads battery percentage using ioreg on macOS.
func readBatteryFromSystem() (float64, bool) {
	cmd := exec.Command("pmset", "-g", "batt")
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}

	output := string(out)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Look for percentage in lines like "Now drawing from 'Battery Power'\n -InternalBattery-0 (id=...)	85%; ..."
		if strings.Contains(line, "%") {
			parts := strings.Fields(line)
			for _, part := range parts {
				part = strings.TrimSuffix(part, "%;")
				part = strings.TrimSuffix(part, "%")
				percent, err := strconv.ParseFloat(part, 64)
				if err == nil && percent >= 0 && percent <= 100 {
					return percent, true
				}
			}
		}
	}

	return 0, false
}

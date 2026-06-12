package scheduler

import (
	"time"

	"github.com/grid-computing/control-plane/internal/domain"
)

// IsInWindow reports whether the current time falls within any of the org's
// configured execution windows. If no windows are configured, it returns true
// (unrestricted scheduling).
func IsInWindow(policy domain.OrgPolicy) bool {
	if len(policy.ExecutionWindows) == 0 {
		return true
	}

	now := time.Now().UTC()

	for _, w := range policy.ExecutionWindows {
		if inWindow(now, w) {
			return true
		}
	}

	return false
}

// inWindow checks if t is within a single TimeWindow.
func inWindow(t time.Time, w domain.TimeWindow) bool {
	// Parse timezone
	loc := time.UTC
	if w.Timezone != "" {
		if l, err := time.LoadLocation(w.Timezone); err == nil {
			loc = l
		}
	}

	local := t.In(loc)
	weekday := int(local.Weekday()) // 0=Sun, 1=Mon, ..., 6=Sat
	hour := local.Hour()

	// Check day of week
	if len(w.DayOfWeek) > 0 && !containsInt(w.DayOfWeek, weekday) {
		return false
	}

	// Handle windows that wrap midnight (e.g., StartHour=22, EndHour=6)
	if w.StartHour <= w.EndHour {
		// Normal window: e.g., 8 → 18
		return hour >= w.StartHour && hour < w.EndHour
	}

	// Wrapping window: e.g., 22 → 6
	return hour >= w.StartHour || hour < w.EndHour
}

func containsInt(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

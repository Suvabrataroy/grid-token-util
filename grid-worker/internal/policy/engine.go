package policy

import (
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/grid-computing/grid-worker/internal/config"
	"github.com/grid-computing/grid-worker/internal/controlplane"
)

// Engine evaluates task execution policy.
type Engine struct {
	cfg *config.Config
	log zerolog.Logger
}

// Decision is the outcome of a policy evaluation.
type Decision struct {
	Allow  bool
	Reason string
}

// New creates a new policy Engine.
func New(cfg *config.Config, log zerolog.Logger) *Engine {
	return &Engine{
		cfg: cfg,
		log: log.With().Str("component", "policy-engine").Logger(),
	}
}

// Evaluate determines whether a task should be executed based on current policy.
func (e *Engine) Evaluate(task *controlplane.TaskAssignment) Decision {
	mode := e.cfg.Policy.Mode

	// 1. Paused mode: deny all tasks
	if mode == "paused" {
		return Decision{Allow: false, Reason: "worker is in paused mode"}
	}

	// 2. Check agent is in permitted list
	if len(e.cfg.Execution.Agents) > 0 {
		permitted := false
		for _, a := range e.cfg.Execution.Agents {
			if a == task.AIAgent {
				permitted = true
				break
			}
		}
		if !permitted {
			return Decision{
				Allow:  false,
				Reason: fmt.Sprintf("agent %q is not in permitted agents list", task.AIAgent),
			}
		}
	}

	// 3. Manual mode: deny unless explicitly approved
	if mode == "manual" {
		return Decision{
			Allow:  false,
			Reason: "worker is in manual approval mode — use 'grid-worker approve <task-id>'",
		}
	}

	// 4. Auto mode: check time windows if any are configured
	if mode == "auto" && len(e.cfg.Policy.Windows) > 0 {
		if !e.IsInWindow() {
			return Decision{
				Allow:  false,
				Reason: "current time is outside all configured execution windows",
			}
		}
	}

	return Decision{Allow: true, Reason: "policy allows task execution"}
}

// IsInWindow returns true if the current time falls within any configured time window.
func (e *Engine) IsInWindow() bool {
	if len(e.cfg.Policy.Windows) == 0 {
		return true
	}

	now := time.Now()

	for _, window := range e.cfg.Policy.Windows {
		if inWindow(now, window) {
			return true
		}
	}

	return false
}

// inWindow checks whether t falls within the given TimeWindowConfig.
func inWindow(t time.Time, window config.TimeWindowConfig) bool {
	loc := time.UTC
	if window.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(window.Timezone)
		if err != nil {
			// Invalid timezone; skip this window
			return false
		}
	}

	local := t.In(loc)
	weekday := int(local.Weekday())
	hour := local.Hour()

	// Check day of week (empty means all days)
	if len(window.DayOfWeek) > 0 {
		dayAllowed := false
		for _, d := range window.DayOfWeek {
			if d == weekday {
				dayAllowed = true
				break
			}
		}
		if !dayAllowed {
			return false
		}
	}

	// Check hour range [StartHour, EndHour)
	startH := window.StartHour
	endH := window.EndHour

	if startH <= endH {
		// Normal range (e.g., 9-17)
		return hour >= startH && hour < endH
	}

	// Wrapping range (e.g., 22-6, overnight)
	return hour >= startH || hour < endH
}

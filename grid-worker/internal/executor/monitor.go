package executor

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

const monitorInterval = 5 * time.Second

// Monitor watches CPU and RAM usage during task execution and invokes
// a kill function if either limit is exceeded for 2 consecutive checks.
type Monitor struct {
	maxCPUPercent float64
	maxRAMMB      int
	killFn        func()
	log           zerolog.Logger
}

// NewMonitor creates a new execution resource Monitor.
func NewMonitor(maxCPUPercent float64, maxRAMMB int, killFn func(), log zerolog.Logger) *Monitor {
	return &Monitor{
		maxCPUPercent: maxCPUPercent,
		maxRAMMB:      maxRAMMB,
		killFn:        killFn,
		log:           log.With().Str("component", "exec-monitor").Logger(),
	}
}

// Start begins monitoring system resources. It polls every 5 seconds and calls
// killFn if CPU or RAM limits are exceeded for 2 consecutive checks.
// Blocks until ctx is cancelled.
func (m *Monitor) Start(ctx context.Context) {
	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()

	cpuViolations := 0
	ramViolations := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cpuOk := m.checkCPU(&cpuViolations)
			ramOk := m.checkRAM(&ramViolations)

			if !cpuOk || !ramOk {
				return // killFn already called
			}
		}
	}
}

// checkCPU measures current CPU usage and increments violations if over limit.
// Returns false if killFn was triggered.
func (m *Monitor) checkCPU(violations *int) bool {
	if m.maxCPUPercent <= 0 {
		return true
	}

	// Sample CPU over 1 second
	percents, err := cpu.Percent(time.Second, false)
	if err != nil || len(percents) == 0 {
		return true
	}

	avgCPU := percents[0]

	if avgCPU > m.maxCPUPercent {
		*violations++
		m.log.Warn().
			Float64("cpu_percent", avgCPU).
			Float64("max_cpu_percent", m.maxCPUPercent).
			Int("violations", *violations).
			Msg("CPU usage exceeds limit")

		if *violations >= 2 {
			m.log.Error().
				Float64("cpu_percent", avgCPU).
				Msg("CPU limit exceeded for 2 consecutive checks, killing task")
			m.killFn()
			return false
		}
	} else {
		*violations = 0
	}

	return true
}

// checkRAM measures current RAM usage and increments violations if over limit.
// Returns false if killFn was triggered.
func (m *Monitor) checkRAM(violations *int) bool {
	if m.maxRAMMB <= 0 {
		return true
	}

	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return true
	}

	usedMB := int(vmStat.Used / (1 << 20))

	if usedMB > m.maxRAMMB {
		*violations++
		m.log.Warn().
			Int("ram_mb_used", usedMB).
			Int("max_ram_mb", m.maxRAMMB).
			Int("violations", *violations).
			Msg("RAM usage exceeds limit")

		if *violations >= 2 {
			m.log.Error().
				Int("ram_mb_used", usedMB).
				Msg("RAM limit exceeded for 2 consecutive checks, killing task")
			m.killFn()
			return false
		}
	} else {
		*violations = 0
	}

	return true
}

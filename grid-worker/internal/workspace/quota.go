package workspace

import (
	"context"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

const quotaCheckInterval = 10 * time.Second

// QuotaMonitor watches a workspace directory and invokes a kill function
// when the total disk usage exceeds the configured limit.
type QuotaMonitor struct {
	workspacePath string
	maxGB         float64
	killFn        func()
	log           zerolog.Logger
}

// NewQuotaMonitor creates a new QuotaMonitor.
func NewQuotaMonitor(workspacePath string, maxGB float64, killFn func(), log zerolog.Logger) *QuotaMonitor {
	return &QuotaMonitor{
		workspacePath: workspacePath,
		maxGB:         maxGB,
		killFn:        killFn,
		log:           log.With().Str("component", "quota-monitor").Logger(),
	}
}

// Start begins monitoring the workspace directory for quota violations.
// It polls every 10 seconds and calls killFn if the quota is exceeded.
// Blocks until ctx is cancelled.
func (q *QuotaMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(quotaCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			usageGB, err := q.measureUsageGB()
			if err != nil {
				q.log.Warn().Err(err).Msg("cannot measure workspace disk usage")
				continue
			}

			q.log.Debug().
				Float64("usage_gb", usageGB).
				Float64("max_gb", q.maxGB).
				Msg("quota check")

			if usageGB > q.maxGB {
				q.log.Error().
					Float64("usage_gb", usageGB).
					Float64("max_gb", q.maxGB).
					Msg("workspace disk quota exceeded, killing task")
				q.killFn()
				return
			}
		}
	}
}

// measureUsageGB computes the total size of all files under the workspace directory.
func (q *QuotaMonitor) measureUsageGB() (float64, error) {
	var totalBytes int64

	err := filepath.WalkDir(q.workspacePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible
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
		return 0, err
	}

	return float64(totalBytes) / (1 << 30), nil
}

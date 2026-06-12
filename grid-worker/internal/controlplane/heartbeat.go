package controlplane

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/rs/zerolog"
)

// HeartbeatLoop manages the periodic heartbeat to the control plane.
type HeartbeatLoop struct {
	client      *Client
	statsFn     func() HeartbeatRequest
	onTask      func(*TaskAssignment)
	intervalSec int
	log         zerolog.Logger
}

// NewHeartbeatLoop creates a HeartbeatLoop.
// statsFn is called on each tick to collect current worker metrics.
// onTask is called when the control plane assigns a task.
func NewHeartbeatLoop(
	client *Client,
	statsFn func() HeartbeatRequest,
	onTask func(*TaskAssignment),
	intervalSec int,
	log zerolog.Logger,
) *HeartbeatLoop {
	if intervalSec <= 0 {
		intervalSec = 30
	}
	return &HeartbeatLoop{
		client:      client,
		statsFn:     statsFn,
		onTask:      onTask,
		intervalSec: intervalSec,
		log:         log.With().Str("component", "heartbeat-loop").Logger(),
	}
}

// Start begins the heartbeat loop. It blocks until ctx is cancelled.
// On 401: logs a fatal message and cancels the context to trigger preflight re-run.
// On 5xx: applies exponential backoff up to 5 minutes.
func (h *HeartbeatLoop) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(h.intervalSec) * time.Second)
	defer ticker.Stop()

	// Send first heartbeat immediately
	h.tick(ctx)

	backoff := time.Duration(0)
	const maxBackoff = 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			h.log.Info().Msg("heartbeat loop stopping")
			return

		case <-ticker.C:
			if backoff > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
			}

			err := h.tick(ctx)
			if err == nil {
				backoff = 0
				continue
			}

			var authErr *AuthError
			if errors.As(err, &authErr) {
				h.log.Fatal().
					Err(err).
					Msg("authentication failed during heartbeat — API key may be revoked; triggering preflight re-run")
				// Cancel context to signal the daemon to re-run preflight
				return
			}

			var serverErr *ServerError
			if errors.As(err, &serverErr) {
				if backoff == 0 {
					backoff = 5 * time.Second
				} else {
					backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
				}
				h.log.Warn().
					Err(err).
					Dur("backoff", backoff).
					Msg("server error during heartbeat, backing off")
				continue
			}

			h.log.Error().Err(err).Msg("heartbeat error")
		}
	}
}

// tick performs a single heartbeat request.
func (h *HeartbeatLoop) tick(ctx context.Context) error {
	req := h.statsFn()

	resp, err := h.client.Heartbeat(ctx, &req)
	if err != nil {
		return err
	}

	if resp.AssignedTask != nil {
		h.log.Info().
			Str("task_id", resp.AssignedTask.TaskID).
			Str("task_type", resp.AssignedTask.TaskType).
			Str("agent", resp.AssignedTask.AIAgent).
			Msg("task assigned by control plane")

		go h.onTask(resp.AssignedTask)
	}

	return nil
}

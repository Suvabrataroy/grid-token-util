package reporter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/grid-computing/grid-worker/internal/controlplane"
)

// Reporter submits packed task output to the control plane.
type Reporter struct {
	client *controlplane.Client
	log    zerolog.Logger
}

// New creates a new Reporter.
func New(client *controlplane.Client, log zerolog.Logger) *Reporter {
	return &Reporter{
		client: client,
		log:    log.With().Str("component", "reporter").Logger(),
	}
}

// Submit sends an OutputPackage to the control plane, retrying up to 3 times
// with exponential backoff (1s, 2s, 4s) on transient failures.
// On 4xx errors (permanent failure), no retry is performed.
func (r *Reporter) Submit(ctx context.Context, pkg *OutputPackage) error {
	// Convert our OutputPackage to the controlplane's OutputSubmission
	artifacts := make([]controlplane.ArtifactMeta, 0, len(pkg.Artifacts))
	for _, a := range pkg.Artifacts {
		artifacts = append(artifacts, controlplane.ArtifactMeta{
			RelPath: a.RelPath,
			SHA256:  a.SHA256,
			Size:    a.Size,
		})
	}

	submission := &controlplane.OutputSubmission{
		TaskID:      pkg.TaskID,
		HMACSha256:  pkg.HMACSha256,
		Artifacts:   artifacts,
		Metadata:    pkg.Metadata,
		XHMACHeader: pkg.HMACSha256,
	}

	const maxRetries = 3
	backoffs := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := r.client.SubmitOutput(ctx, submission)
		if err == nil {
			r.log.Info().
				Str("task_id", pkg.TaskID).
				Int("artifacts", len(pkg.Artifacts)).
				Msg("output submitted successfully")
			return nil
		}

		// Check if it's a permanent client error (4xx)
		var serverErr *controlplane.ServerError
		if !errors.As(err, &serverErr) {
			// Likely a 4xx or client-side error — no retry
			r.log.Error().
				Err(err).
				Str("task_id", pkg.TaskID).
				Msg("output submission failed with permanent error, no retry")
			return fmt.Errorf("submit output (permanent): %w", err)
		}

		lastErr = err
		r.log.Warn().
			Err(err).
			Str("task_id", pkg.TaskID).
			Int("attempt", attempt+1).
			Int("max_retries", maxRetries).
			Msg("output submission failed, will retry")

		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoffs[attempt]):
			}
		}
	}

	return fmt.Errorf("submit output after %d retries: %w", maxRetries, lastErr)
}

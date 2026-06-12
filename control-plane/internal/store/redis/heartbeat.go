package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const heartbeatKeyPrefix = "worker:heartbeat:"

func heartbeatKey(workerID uuid.UUID) string {
	return heartbeatKeyPrefix + workerID.String()
}

// SetWorkerHeartbeat records a heartbeat for the given worker with an explicit
// TTL.  The key expires automatically when no heartbeat is received within the
// TTL window, allowing the reaper to detect offline workers.
func (r *RedisStore) SetWorkerHeartbeat(ctx context.Context, workerID uuid.UUID, ttlSec int) error {
	key := heartbeatKey(workerID)
	if err := r.client.Set(ctx, key, "1", time.Duration(ttlSec)*time.Second).Err(); err != nil {
		return fmt.Errorf("redis: set heartbeat %s: %w", workerID, err)
	}
	return nil
}

// GetWorkerHeartbeat returns true if a heartbeat key exists for the worker
// (i.e., the worker is considered alive).
func (r *RedisStore) GetWorkerHeartbeat(ctx context.Context, workerID uuid.UUID) (bool, error) {
	n, err := r.client.Exists(ctx, heartbeatKey(workerID)).Result()
	if err != nil {
		return false, fmt.Errorf("redis: exists heartbeat %s: %w", workerID, err)
	}
	return n > 0, nil
}

// DeleteWorkerHeartbeat removes the heartbeat key immediately, e.g. when a
// worker gracefully shuts down.
func (r *RedisStore) DeleteWorkerHeartbeat(ctx context.Context, workerID uuid.UUID) error {
	if err := r.client.Del(ctx, heartbeatKey(workerID)).Err(); err != nil {
		return fmt.Errorf("redis: del heartbeat %s: %w", workerID, err)
	}
	return nil
}

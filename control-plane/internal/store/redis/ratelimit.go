package redis

import (
	"context"
	"fmt"
	"time"
)

// RateLimitIncr atomically increments a sliding-window counter in Redis.
// On the first call within the window it also sets the expiry so that the key
// is automatically garbage-collected when the window elapses.
// Returns the new counter value after incrementing.
func (r *RedisStore) RateLimitIncr(ctx context.Context, key string, window time.Duration) (int64, error) {
	pipe := r.client.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("redis: ratelimit incr %s: %w", key, err)
	}

	return incrCmd.Val(), nil
}

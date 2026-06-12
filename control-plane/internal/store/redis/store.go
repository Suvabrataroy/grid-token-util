// Package redis provides the Redis-backed caching and ephemeral-state layer.
package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"github.com/grid-computing/control-plane/internal/config"
)

// RedisStore wraps a go-redis client and exposes the operations used by the
// control plane.
type RedisStore struct {
	client *goredis.Client
}

// New creates a Redis client and verifies connectivity.
func New(cfg config.RedisConfig) (*RedisStore, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return &RedisStore{client: client}, nil
}

// Close shuts down the Redis connection pool.
func (r *RedisStore) Close() error {
	return r.client.Close()
}

// Ping checks that Redis is reachable.
func (r *RedisStore) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: ping: %w", err)
	}
	return nil
}

// Client exposes the underlying go-redis client for use by sub-stores.
func (r *RedisStore) Client() *goredis.Client {
	return r.client
}

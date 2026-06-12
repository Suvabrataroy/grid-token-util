// Package queue provides a Redis Sorted Set-backed task queue.
package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

const taskQueueKey = "grid:task_queue"

// RedisQueue is a priority queue backed by a Redis Sorted Set.
// The score combines priority and submission timestamp so that higher-priority
// tasks are returned first, with FIFO ordering within the same priority tier.
type RedisQueue struct {
	client *goredis.Client
}

// New creates a RedisQueue using the supplied go-redis client.
func New(client *goredis.Client) *RedisQueue {
	return &RedisQueue{client: client}
}

// Enqueue adds a task to the queue.
//
// Score formula: score = priority * 1e10 - unixMicro(queuedAt)
//
// Subtracting the timestamp means that for equal priorities, earlier-queued
// tasks get a higher score (FIFO). ZPOPMAX is used in Dequeue to retrieve the
// highest score (= highest priority + earliest queued).
func (q *RedisQueue) Enqueue(ctx context.Context, taskID uuid.UUID, priority int, queuedAt time.Time) error {
	score := float64(priority)*1e10 - float64(queuedAt.UnixMicro())
	if err := q.client.ZAdd(ctx, taskQueueKey, goredis.Z{
		Score:  score,
		Member: taskID.String(),
	}).Err(); err != nil {
		return fmt.Errorf("queue: enqueue %s: %w", taskID, err)
	}
	return nil
}

// Dequeue atomically removes and returns the task with the highest score.
// Returns nil without error when the queue is empty.
func (q *RedisQueue) Dequeue(ctx context.Context) (*uuid.UUID, error) {
	results, err := q.client.ZPopMax(ctx, taskQueueKey, 1).Result()
	if err != nil {
		return nil, fmt.Errorf("queue: dequeue: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}

	idStr, ok := results[0].Member.(string)
	if !ok {
		return nil, fmt.Errorf("queue: unexpected member type: %T", results[0].Member)
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("queue: parse task id %q: %w", idStr, err)
	}
	return &id, nil
}

// QueueLength returns the number of tasks currently waiting in the queue.
func (q *RedisQueue) QueueLength(ctx context.Context) (int64, error) {
	n, err := q.client.ZCard(ctx, taskQueueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("queue: length: %w", err)
	}
	return n, nil
}

// RemoveFromQueue removes a specific task from the queue, e.g. when a task is
// cancelled before being picked up.
func (q *RedisQueue) RemoveFromQueue(ctx context.Context, taskID uuid.UUID) error {
	if err := q.client.ZRem(ctx, taskQueueKey, taskID.String()).Err(); err != nil {
		return fmt.Errorf("queue: remove %s: %w", taskID, err)
	}
	return nil
}

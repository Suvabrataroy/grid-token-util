package dashboard

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const (
	streamKeyPrefix = "dashboard:events:"
	consumerGroup   = "sse-projector"
	maxPendingAge   = 5 * time.Minute
)

// Projector reads from a Redis Stream for an org and fans events out to SSE
// clients via the Hub.
type Projector struct {
	rdb   *redis.Client
	hub   *Hub
	orgID uuid.UUID
	log   zerolog.Logger
}

// NewProjector creates a Projector for the specified org unit.
func NewProjector(rdb *redis.Client, hub *Hub, orgID uuid.UUID, log zerolog.Logger) *Projector {
	return &Projector{rdb: rdb, hub: hub, orgID: orgID, log: log}
}

// Run blocks and continuously reads from the Redis Stream for the org unit,
// broadcasting each event to connected SSE clients.  It returns when ctx is
// cancelled.
func (p *Projector) Run(ctx context.Context) {
	streamKey := streamKeyPrefix + p.orgID.String()
	consumerName := "projector-" + p.orgID.String()

	// Create consumer group if it doesn't exist; mkstream creates the stream
	// atomically if absent.
	_ = p.rdb.XGroupCreateMkStream(ctx, streamKey, consumerGroup, "$").Err()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := p.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumerName,
			Streams:  []string{streamKey, ">"},
			Count:    10,
			Block:    1 * time.Second,
		}).Result()

		if err != nil {
			if err != redis.Nil && ctx.Err() == nil {
				p.log.Warn().Err(err).Str("org_id", p.orgID.String()).Msg("projector: XReadGroup error")
				time.Sleep(500 * time.Millisecond)
			}
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				p.processMessage(ctx, msg)
				// ACK so the message is not re-delivered on restart.
				p.rdb.XAck(ctx, streamKey, consumerGroup, msg.ID)
			}
		}
	}
}

// processMessage decodes a Redis stream message and broadcasts it to the Hub.
func (p *Projector) processMessage(_ context.Context, msg redis.XMessage) {
	eventType, _ := msg.Values["type"].(string)
	dataStr, _ := msg.Values["data"].(string)

	var data any
	if dataStr != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(dataStr), &m); err == nil {
			data = m
		} else {
			data = dataStr
		}
	}

	event := Event{
		Type: eventType,
		Data: data,
		ID:   msg.ID,
	}

	p.hub.Broadcast(p.orgID, event)
}

// PublishEvent publishes an event to the Redis Stream for an org unit.  The
// stream is capped at 1000 messages (approximate) to bound memory usage.
func PublishEvent(ctx context.Context, rdb *redis.Client, orgID uuid.UUID, eventType string, data any) error {
	streamKey := streamKeyPrefix + orgID.String()
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: 1000,
		Approx: true,
		Values: map[string]any{
			"type": eventType,
			"data": string(dataJSON),
		},
	}).Err()
}

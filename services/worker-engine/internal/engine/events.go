package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"coldharbor/worker-engine/internal/model"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisEventPublisher broadcasts state transition events to a Redis Pub/Sub channel
type RedisEventPublisher struct {
	client  redis.UniversalClient
	channel string
}

// NewEventPublisher creates a new publisher configured with the target pub/sub channel
func NewEventPublisher(client redis.UniversalClient, channel string) *RedisEventPublisher {
	if channel == "" {
		channel = "coldharbor:events"
	}
	return &RedisEventPublisher{
		client:  client,
		channel: channel,
	}
}

// Publish broadcasts an event to the Redis Pub/Sub channel
func (p *RedisEventPublisher) Publish(ctx context.Context, evt *model.EventMessage) error {
	if evt == nil {
		return nil
	}

	if evt.EventID == "" {
		evt.EventID = fmt.Sprintf("evt_%s", uuid.New().String()[:12])
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}
	// Contract compatibility aliases
	if evt.PreviousState == "" {
		evt.PreviousState = evt.FromState
	}
	if evt.CurrentState == "" {
		evt.CurrentState = evt.ToState
	}
	if evt.Progress == 0 && evt.CheckpointPct > 0 {
		evt.Progress = evt.CheckpointPct
	} else if evt.CheckpointPct == 0 && evt.Progress > 0 {
		evt.CheckpointPct = evt.Progress
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("failed to marshal event message: %w", err)
	}

	if err := p.client.Publish(ctx, p.channel, payload).Err(); err != nil {
		return fmt.Errorf("failed to publish event to channel %s: %w", p.channel, err)
	}

	return nil
}

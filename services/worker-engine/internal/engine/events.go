package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"coldharbor/worker-engine/internal/model"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisEventPublisher broadcasts state transition events to a Redis Pub/Sub channel
type RedisEventPublisher struct {
	client     redis.UniversalClient
	channel    string
	streamName string
	metrics    *Metrics
}

// NewEventPublisher creates a publisher that writes every event to a durable
// stream before attempting best-effort Pub/Sub fanout.
func NewEventPublisher(client redis.UniversalClient, channel string, eventStream ...string) *RedisEventPublisher {
	if channel == "" {
		channel = "coldharbor:events"
	}
	streamName := "coldharbor:events:stream"
	if len(eventStream) > 0 && eventStream[0] != "" {
		streamName = eventStream[0]
	}
	return &RedisEventPublisher{
		client:     client,
		channel:    channel,
		streamName: streamName,
	}
}

// Publish appends an event to the durable stream, then broadcasts it live.
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

	if err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.streamName,
		MaxLen: 100_000,
		Approx: true,
		Values: map[string]any{
			"data":          string(payload),
			"eventId":       evt.EventID,
			"compartmentId": evt.CompartmentID,
			"toState":       string(evt.ToState),
		},
	}).Err(); err != nil {
		if p.metrics != nil {
			p.metrics.IncPublishErrors()
		}
		log.Printf("[events] failed to persist transition for compartment %s: %v", evt.CompartmentID, err)
		return fmt.Errorf("failed to append event to stream %s: %w", p.streamName, err)
	}

	if err := p.client.Publish(ctx, p.channel, payload).Err(); err != nil {
		if p.metrics != nil {
			p.metrics.IncPublishErrors()
		}
		log.Printf("[events] failed to publish transition for compartment %s: %v", evt.CompartmentID, err)
	}

	return nil
}

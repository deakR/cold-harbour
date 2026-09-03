package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"coldharbor/worker-engine/internal/model"
	"github.com/redis/go-redis/v9"
)

// HeartbeatEmitter manages periodic and event-triggered worker liveness reporting
type HeartbeatEmitter struct {
	client              redis.UniversalClient
	workerID            string
	interval            time.Duration
	ttl                 time.Duration
	status              string
	activeCompartmentID string
	mu                  sync.RWMutex
	stopChan            chan struct{}
	wg                  sync.WaitGroup
}

// NewHeartbeatEmitter initializes a heartbeat emitter
func NewHeartbeatEmitter(
	client redis.UniversalClient,
	workerID string,
	interval, ttl time.Duration,
) *HeartbeatEmitter {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &HeartbeatEmitter{
		client:   client,
		workerID: workerID,
		interval: interval,
		ttl:      ttl,
		status:   "IDLE",
		stopChan: make(chan struct{}),
	}
}

// Key returns the Redis key for this worker's heartbeat beacon
func (h *HeartbeatEmitter) Key() string {
	return fmt.Sprintf("worker:%s:heartbeat", h.workerID)
}

// SetStatus updates worker status and active compartment, immediately emitting a heartbeat
func (h *HeartbeatEmitter) SetStatus(status, activeCompartmentID string) {
	h.mu.Lock()
	h.status = status
	h.activeCompartmentID = activeCompartmentID
	h.mu.Unlock()

	_ = h.Beat(context.Background())
}

// Beat writes a single heartbeat payload to Redis with TTL
func (h *HeartbeatEmitter) Beat(ctx context.Context) error {
	h.mu.RLock()
	payload := model.HeartbeatPayload{
		WorkerID:            h.workerID,
		Status:              h.status,
		ActiveCompartmentID: h.activeCompartmentID,
		Timestamp:           time.Now().UTC(),
	}
	h.mu.RUnlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat payload: %w", err)
	}

	key := h.Key()
	if err := h.client.Set(ctx, key, data, h.ttl).Err(); err != nil {
		return fmt.Errorf("failed to write heartbeat to %s: %w", key, err)
	}

	return nil
}

// Start initiates the periodic heartbeat ticker in a background goroutine
func (h *HeartbeatEmitter) Start(ctx context.Context) {
	// Emit initial heartbeat immediately
	_ = h.Beat(ctx)

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_ = h.Beat(ctx)
			case <-h.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop terminates the periodic heartbeat ticker and waits for goroutine completion
func (h *HeartbeatEmitter) Stop() {
	select {
	case <-h.stopChan:
		// already stopped
		return
	default:
		close(h.stopChan)
	}
	h.wg.Wait()
}

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"coldharbor/worker-engine/internal/config"
	"coldharbor/worker-engine/internal/model"
	"github.com/redis/go-redis/v9"
)

// Consumer orchestrates the Redis Stream consumer group loop, task processing,
// checkpointing, dead drop archive sealing, and atomic scratchpad purge.
type Consumer struct {
	cfg        *config.Config
	client     redis.UniversalClient
	scratchpad *ScratchpadManager
	archive    *ArchiveManager
	publisher  EventPublisher
	dlq        *DLQHandler
	recovery   *RecoveryManager
	heartbeat  *HeartbeatEmitter
	runner     *TaskRunner
	stopChan   chan struct{}
	wg         sync.WaitGroup
}

// NewConsumer initializes the consumer engine with all required subsystems
func NewConsumer(cfg *config.Config, client redis.UniversalClient) *Consumer {
	scratchpad := NewScratchpadManager(client)
	archive := NewArchiveManager(client, cfg.ArchiveTTL)
	publisher := NewEventPublisher(client, cfg.EventChannel)
	dlq := NewDLQHandler(client, cfg.DLQStreamName, scratchpad, publisher)
	recovery := NewRecoveryManager(client, cfg.StreamName, cfg.ConsumerGroup, cfg.WorkerID, cfg.MinIdleRecoveryTime, scratchpad)
	heartbeat := NewHeartbeatEmitter(client, cfg.WorkerID, cfg.HeartbeatInterval, cfg.HeartbeatTTL)
	runner := NewTaskRunner(scratchpad)

	return &Consumer{
		cfg:        cfg,
		client:     client,
		scratchpad: scratchpad,
		archive:    archive,
		publisher:  publisher,
		dlq:        dlq,
		recovery:   recovery,
		heartbeat:  heartbeat,
		runner:     runner,
		stopChan:   make(chan struct{}),
	}
}

// InitConsumerGroup initializes the Redis Stream and consumer group if not already present
func (c *Consumer) InitConsumerGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, "0").Err()
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "busygroup") {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}
	return nil
}

// ParseStreamMessage extracts a JobMessage from Redis stream message values,
// accommodating both unified JSON data blobs and discrete field maps.
func ParseStreamMessage(xmsg redis.XMessage) (*model.JobMessage, error) {
	values := xmsg.Values
	if values == nil {
		return nil, errors.New("stream message values are nil")
	}

	// 1. Unified JSON string in "data" or "job"
	for _, k := range []string{"data", "job", "message"} {
		if raw, ok := values[k]; ok {
			var jsonBytes []byte
			switch v := raw.(type) {
			case string:
				jsonBytes = []byte(v)
			case []byte:
				jsonBytes = v
			}
			if len(jsonBytes) > 0 {
				var job model.JobMessage
				if err := json.Unmarshal(jsonBytes, &job); err == nil && job.CompartmentID != "" {
					return &job, nil
				}
			}
		}
	}

	// 2. Discrete field mapping
	job := &model.JobMessage{
		Payload: make(map[string]any),
	}

	if cid, ok := values["compartmentId"].(string); ok {
		job.CompartmentID = cid
	}
	if ctx, ok := values["context"].(string); ok {
		job.Context = ctx
	}
	if oid, ok := values["ownerId"].(string); ok {
		job.OwnerID = oid
	}
	if tt, ok := values["taskType"].(string); ok {
		job.TaskType = tt
	}

	if retries, ok := values["maxRetries"]; ok {
		switch r := retries.(type) {
		case int:
			job.MaxRetries = r
		case string:
			job.MaxRetries, _ = strconv.Atoi(r)
		}
	}

	if timeout, ok := values["timeoutSeconds"]; ok {
		switch to := timeout.(type) {
		case int:
			job.TimeoutSeconds = to
		case string:
			job.TimeoutSeconds, _ = strconv.Atoi(to)
		}
	}

	if p, ok := values["payload"]; ok {
		switch pv := p.(type) {
		case string:
			var pMap map[string]any
			if err := json.Unmarshal([]byte(pv), &pMap); err == nil {
				job.Payload = pMap
			} else {
				job.Payload["raw"] = pv
			}
		case map[string]any:
			job.Payload = pv
		}
	}

	if ca, ok := values["createdAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339, ca); err == nil {
			job.CreatedAt = t
		}
	}

	if job.CompartmentID == "" {
		return nil, fmt.Errorf("message %s lacks compartmentId", xmsg.ID)
	}

	return job, nil
}

// ProcessJob executes the complete lifecycle of a single claimed job
func (c *Consumer) ProcessJob(ctx context.Context, xmsg redis.XMessage) error {
	job, err := ParseStreamMessage(xmsg)
	if err != nil {
		log.Printf("[worker %s] failed to parse message %s: %v", c.cfg.WorkerID, xmsg.ID, err)
		// Poison pill: route to DLQ immediately
		return c.dlq.RouteToDLQ(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID, c.cfg.WorkerID,
			&model.JobMessage{CompartmentID: fmt.Sprintf("unparseable-%s", xmsg.ID)},
			1, fmt.Sprintf("message parse error: %v", err))
	}

	compartmentID := job.CompartmentID
	c.heartbeat.SetStatus("BUSY", compartmentID)
	defer c.heartbeat.SetStatus("IDLE", "")

	maxRetries := c.cfg.MaxRetries
	if job.MaxRetries > 0 {
		maxRetries = job.MaxRetries
	}

	// 1. Check current retry count
	retries, _ := c.dlq.GetRetryCount(ctx, compartmentID)
	if retries >= maxRetries {
		log.Printf("[worker %s] compartment %s exceeded max retries (%d/%d), forwarding to DLQ",
			c.cfg.WorkerID, compartmentID, retries, maxRetries)
		return c.dlq.RouteToDLQ(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID, c.cfg.WorkerID,
			job, retries, "max retries exceeded")
	}

	// 2. FSM & Event Publisher: QUEUED -> RUNNING
	fsm := NewFSM(compartmentID, c.cfg.WorkerID, model.StateQueued, c.publisher)
	if err := fsm.Transition(ctx, model.StateRunning, 0, "Worker claimed job from stream"); err != nil {
		return fmt.Errorf("failed to transition to RUNNING: %w", err)
	}

	// 3. Initialize Checkpoint Recorder & Execute Task
	recorder := NewCheckpointRecorder(c.scratchpad, fsm)
	taskRes, err := c.runner.Execute(ctx, job, recorder)
	if err != nil {
		log.Printf("[worker %s] execution error on compartment %s: %v", c.cfg.WorkerID, compartmentID, err)
		newRetries, _ := c.dlq.IncrementRetryCount(ctx, compartmentID)

		if strings.Contains(err.Error(), "SIMULATED_WORKER_CRASH") {
			// Simulated crash for recovery test: do not ack, do not purge, leave for PEL recovery
			return err
		}

		if newRetries >= maxRetries {
			return c.dlq.RouteToDLQ(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID, c.cfg.WorkerID,
				job, newRetries, err.Error())
		}

		// Transient failure: retries remaining. Do not emit StateFailed to avoid premature terminal failure events.
		// Leave unacknowledged in stream / PEL for next retry attempt.
		return err
	}

	// 4. Task completed successfully -> COMPLETED
	if err := fsm.Transition(ctx, model.StateCompleted, 100, "Task logic completed successfully"); err != nil {
		return fmt.Errorf("failed to transition to COMPLETED: %w", err)
	}

	// 5. Dead Drop Sealing -> ARCHIVED
	deadDrop := &model.DeadDropPayload{
		CompartmentID: compartmentID,
		OwnerID:       job.OwnerID,
		Context:       job.Context,
		TaskType:      job.TaskType,
		Output:        taskRes.Output,
		DurationMs:    taskRes.DurationMs,
		TTLSeconds:    int(c.cfg.ArchiveTTL.Seconds()),
	}
	if err := c.archive.Seal(ctx, deadDrop); err != nil {
		return fmt.Errorf("failed to seal dead drop archive: %w", err)
	}

	if err := fsm.Transition(ctx, model.StateArchived, 100, fmt.Sprintf("Dead drop sealed with checksum %s", deadDrop.Checksum)); err != nil {
		return fmt.Errorf("failed to transition to ARCHIVED: %w", err)
	}

	// 6. Ephemeral Scratchpad Purge (Zero-Leak Guarantee) -> PURGED
	if err := c.scratchpad.Purge(ctx, compartmentID); err != nil {
		return fmt.Errorf("failed to purge scratchpad: %w", err)
	}

	// Verify invariant: EXISTS == 0
	exists, err := c.scratchpad.Exists(ctx, compartmentID)
	if err != nil || exists {
		return fmt.Errorf("zero-leak invariant violated: scratchpad for %s still exists post-purge", compartmentID)
	}

	if err := fsm.Transition(ctx, model.StatePurged, 100, "Scratchpad memory purged atomically"); err != nil {
		return fmt.Errorf("failed to transition to PURGED: %w", err)
	}

	// 7. Deferred XACK only after successful archive and scratchpad purge
	if err := c.client.XAck(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID).Err(); err != nil {
		return fmt.Errorf("failed to XACK message %s: %w", xmsg.ID, err)
	}

	// 8. Clean up retry counter
	_ = c.client.Del(ctx, c.dlq.RetryKey(compartmentID)).Err()

	log.Printf("[worker %s] successfully completed, sealed, purged, and ACKed compartment %s (checksum: %s)",
		c.cfg.WorkerID, compartmentID, deadDrop.Checksum)
	return nil
}

// Start begins consuming jobs from the Redis Stream and periodic recovery
func (c *Consumer) Start(ctx context.Context) {
	c.heartbeat.Start(ctx)

	// Recovery routine
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.cfg.RecoveryInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.runRecoveryPass(ctx)
			case <-c.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Main Consumer Loop
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-c.stopChan:
				return
			case <-ctx.Done():
				return
			default:
				// Read new messages with XREADGROUP
				streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
					Group:    c.cfg.ConsumerGroup,
					Consumer: c.cfg.WorkerID,
					Streams:  []string{c.cfg.StreamName, ">"},
					Count:    int64(c.cfg.Concurrency),
					Block:    c.cfg.BlockDuration,
				}).Result()

				if err != nil {
					if err == redis.Nil || strings.Contains(strings.ToLower(err.Error()), "timeout") {
						continue
					}
					select {
					case <-c.stopChan:
						return
					case <-ctx.Done():
						return
					default:
						time.Sleep(200 * time.Millisecond)
						continue
					}
				}

				for _, stream := range streams {
					for _, msg := range stream.Messages {
						_ = c.ProcessJob(ctx, msg)
					}
				}
			}
		}
	}()
}

// runRecoveryPass claims abandoned pending entries and resumes execution
func (c *Consumer) runRecoveryPass(ctx context.Context) {
	msgs, err := c.recovery.ClaimPendingJobs(ctx, int64(c.cfg.Concurrency))
	if err != nil {
		return
	}

	for _, msg := range msgs {
		_ = c.ProcessJob(ctx, msg)
	}
}

// Stop gracefully stops consumer routines and the heartbeat emitter
func (c *Consumer) Stop() {
	select {
	case <-c.stopChan:
		return
	default:
		close(c.stopChan)
	}
	c.heartbeat.Stop()
	c.wg.Wait()
}

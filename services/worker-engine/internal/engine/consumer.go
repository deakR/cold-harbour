package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	completion *CompletionGuard
	publisher  EventPublisher
	dlq        *DLQHandler
	recovery   *RecoveryManager
	heartbeat  *HeartbeatEmitter
	runner     TaskExecutor
	jobs       chan redis.XMessage
	stopChan   chan struct{}
	runCancel  context.CancelFunc
	wg         sync.WaitGroup
	metrics    *Metrics
}

// NewConsumer initializes the consumer engine with all required subsystems
func NewConsumer(cfg *config.Config, client redis.UniversalClient) *Consumer {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = config.DefaultConfig().MaxRetries
	}
	metrics := &Metrics{}
	scratchpad := NewScratchpadManager(client)
	archive := NewArchiveManager(client, cfg.ArchiveTTL)
	publisher := NewEventPublisher(client, cfg.EventChannel, cfg.EventStreamName)
	publisher.metrics = metrics
	dlq := NewDLQHandler(client, cfg.DLQStreamName, scratchpad, publisher, cfg.AuditStreamName)
	recovery := NewRecoveryManager(client, cfg.StreamName, cfg.ConsumerGroup, cfg.WorkerID, cfg.MinIdleRecoveryTime, scratchpad)
	heartbeat := NewHeartbeatEmitter(client, cfg.WorkerID, cfg.HeartbeatInterval, cfg.HeartbeatTTL)
	runner := NewTaskRunner(scratchpad)

	return &Consumer{
		cfg:        cfg,
		client:     client,
		scratchpad: scratchpad,
		archive:    archive,
		completion: NewCompletionGuard(client, cfg.ArchiveTTL),
		publisher:  publisher,
		dlq:        dlq,
		recovery:   recovery,
		heartbeat:  heartbeat,
		runner:     runner,
		jobs:       make(chan redis.XMessage, cfg.Concurrency),
		stopChan:   make(chan struct{}),
		metrics:    metrics,
	}
}

// MetricsSnapshot exposes the consumer counters for HTTP exposition.
func (c *Consumer) MetricsSnapshot() *Metrics {
	if c.metrics == nil {
		return &Metrics{}
	}
	return c.metrics
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
		if r, valid := numericInt64(retries); valid && r >= 0 && r <= int64(^uint(0)>>1) {
			job.MaxRetries = int(r)
		}
	}

	if timeout, ok := values["timeoutSeconds"]; ok {
		if seconds, valid := numericInt64(timeout); valid && seconds >= 0 && seconds <= int64(^uint(0)>>1) {
			job.TimeoutSeconds = int(seconds)
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
	c.metrics.IncClaimed()
	job, err := ParseStreamMessage(xmsg)
	if err != nil {
		log.Printf("[worker %s] failed to parse message %s: %v", c.cfg.WorkerID, xmsg.ID, err)
		c.metrics.IncFailed()
		c.metrics.IncDLQRouted()
		// Poison pill: route to DLQ immediately
		return c.dlq.RouteToDLQ(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID, c.cfg.WorkerID,
			&model.JobMessage{CompartmentID: fmt.Sprintf("unparseable-%s", xmsg.ID)},
			1, fmt.Sprintf("message parse error: %v", err))
	}

	compartmentID := job.CompartmentID
	completed, err := c.completion.IsCompleted(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID)
	if err != nil {
		return err
	}
	if completed {
		return c.ackCompletedJob(ctx, xmsg, job)
	}

	leaseTTL := time.Hour
	if job.TimeoutSeconds > 0 {
		leaseTTL = time.Duration(job.TimeoutSeconds)*time.Second + time.Minute
	}
	if minimum := 2 * c.cfg.MinIdleRecoveryTime; leaseTTL < minimum {
		leaseTTL = minimum
	}
	leaseToken, acquired, completed, err := c.completion.Acquire(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID, leaseTTL)
	if err != nil {
		return err
	}
	if completed {
		return c.ackCompletedJob(ctx, xmsg, job)
	}
	if !acquired {
		// Another live processor owns this delivery. Leave it pending; the
		// completion marker or recovery pass will safely ACK it later.
		return nil
	}
	leaseHeld := true
	defer func() {
		if !leaseHeld {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.completion.Release(releaseCtx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID, leaseToken); err != nil {
			log.Printf("[worker %s] failed to release lease for message %s: %v", c.cfg.WorkerID, xmsg.ID, err)
		}
	}()

	c.heartbeat.SetStatus("BUSY", compartmentID)
	defer c.heartbeat.SetStatus("IDLE", "")

	maxRetries := c.cfg.MaxRetries
	if job.MaxRetries > 0 {
		maxRetries = job.MaxRetries
	}

	// 1. Check current retry count
	retries, err := c.dlq.GetRetryCount(ctx, compartmentID)
	if err != nil {
		return err
	}
	if retries >= maxRetries {
		log.Printf("[worker %s] compartment %s exceeded max retries (%d/%d), forwarding to DLQ",
			c.cfg.WorkerID, compartmentID, retries, maxRetries)
		c.metrics.IncFailed()
		c.metrics.IncDLQRouted()
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
	executionCtx := ctx
	cancelExecution := func() {}
	if job.TimeoutSeconds > 0 {
		executionCtx, cancelExecution = context.WithTimeout(ctx, time.Duration(job.TimeoutSeconds)*time.Second)
	}
	taskRes, err := c.runner.Execute(executionCtx, job, recorder)
	cancelExecution()
	if err != nil {
		log.Printf("[worker %s] execution error on compartment %s: %v", c.cfg.WorkerID, compartmentID, err)
		c.metrics.IncFailed()

		if strings.Contains(err.Error(), "SIMULATED_WORKER_CRASH") {
			// Simulated crash for recovery test: do not ack, do not purge, leave for PEL recovery
			return err
		}
		if ctx.Err() != nil {
			// Service shutdown/caller cancellation is not a job failure and
			// must not consume retry budget.
			return fmt.Errorf("job execution canceled: %w", err)
		}

		reason := err.Error()
		if job.TimeoutSeconds > 0 && errors.Is(err, context.DeadlineExceeded) {
			reason = fmt.Sprintf("job execution timed out after %d seconds", job.TimeoutSeconds)
			c.metrics.IncTimedOut()
		}
		newRetries, retryErr := c.dlq.IncrementRetryCount(ctx, compartmentID)
		if retryErr != nil {
			return fmt.Errorf("record retry after execution failure (%s): %w", reason, retryErr)
		}

		if newRetries >= maxRetries {
			c.metrics.IncDLQRouted()
			return c.dlq.RouteToDLQ(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID, c.cfg.WorkerID,
				job, newRetries, reason)
		}

		// Transient failure: retries remaining. Do not emit StateFailed to avoid premature terminal failure events.
		// Leave unacknowledged in stream / PEL for next retry attempt.
		return fmt.Errorf("%s: %w", reason, err)
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
	c.metrics.IncSealed()

	if err := fsm.Transition(ctx, model.StateArchived, 100, fmt.Sprintf("Dead drop sealed with checksum %s", deadDrop.Checksum)); err != nil {
		return fmt.Errorf("failed to transition to ARCHIVED: %w", err)
	}

	// 6. Ephemeral Scratchpad Purge (Zero-Leak Guarantee) -> PURGED
	if err := c.completion.Finalize(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID,
		c.scratchpad.Key(compartmentID), leaseToken); err != nil {
		return err
	}
	leaseHeld = false

	// Verify invariant: EXISTS == 0
	exists, err := c.scratchpad.Exists(ctx, compartmentID)
	if err != nil || exists {
		return fmt.Errorf("zero-leak invariant violated: scratchpad for %s still exists post-purge", compartmentID)
	}
	c.metrics.IncPurged()

	if err := c.ensureAuditPublished(ctx, xmsg, deadDrop); err != nil {
		return err
	}

	if err := fsm.Transition(ctx, model.StatePurged, 100, "Scratchpad memory purged atomically"); err != nil {
		return fmt.Errorf("failed to transition to PURGED: %w", err)
	}
	if err := c.completion.MarkTerminalEventPublished(
		ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID,
	); err != nil {
		return err
	}

	// 7. Deferred XACK only after successful archive and scratchpad purge
	if err := c.client.XAck(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID).Err(); err != nil {
		return fmt.Errorf("failed to XACK message %s: %w", xmsg.ID, err)
	}
	c.metrics.IncCompleted()

	// 8. Clean up retry counter
	if err := c.client.Del(ctx, c.dlq.RetryKey(compartmentID)).Err(); err != nil {
		log.Printf("[worker %s] failed to clear retry key for compartment %s: %v", c.cfg.WorkerID, compartmentID, err)
	}

	log.Printf("[worker %s] successfully completed, sealed, purged, and ACKed compartment %s (checksum: %s)",
		c.cfg.WorkerID, compartmentID, deadDrop.Checksum)
	return nil
}

func (c *Consumer) ackCompletedJob(ctx context.Context, xmsg redis.XMessage, job *model.JobMessage) error {
	deadDrop, err := c.archive.Get(ctx, job.CompartmentID)
	if err != nil {
		return fmt.Errorf("retrieve completed archive for audit recovery: %w", err)
	}
	if err := c.ensureAuditPublished(ctx, xmsg, deadDrop); err != nil {
		return err
	}
	needsEvent, err := c.completion.NeedsTerminalEvent(
		ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID,
	)
	if err != nil {
		return err
	}
	if needsEvent {
		if err := c.publisher.Publish(ctx, &model.EventMessage{
			CompartmentID: job.CompartmentID,
			WorkerID:      c.cfg.WorkerID,
			FromState:     model.StateArchived,
			ToState:       model.StatePurged,
			PreviousState: model.StateArchived,
			CurrentState:  model.StatePurged,
			CheckpointPct: 100,
			Progress:      100,
			Timestamp:     time.Now().UTC(),
			Details:       "Recovered durable PURGED event for completed delivery",
		}); err != nil {
			return err
		}
		if err := c.completion.MarkTerminalEventPublished(
			ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID,
		); err != nil {
			return err
		}
	}
	if err := c.client.XAck(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID).Err(); err != nil {
		return fmt.Errorf("failed to XACK completed message %s: %w", xmsg.ID, err)
	}
	c.metrics.IncDeduplicated()
	return nil
}

func (c *Consumer) ensureAuditPublished(
	ctx context.Context, xmsg redis.XMessage, deadDrop *model.DeadDropPayload,
) error {
	published, err := c.completion.IsAuditPublished(
		ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID,
	)
	if err != nil {
		return err
	}
	if published {
		return nil
	}
	if err := appendAuditEnvelope(
		ctx,
		c.client,
		c.cfg.AuditStreamName,
		deadDrop,
		string(model.StatePurged),
		"Scratchpad memory purged atomically",
	); err != nil {
		return err
	}
	return c.completion.MarkAuditPublished(
		ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, xmsg.ID,
	)
}

// Start begins consuming jobs from the Redis Stream and periodic recovery
func (c *Consumer) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	c.runCancel = cancel
	c.heartbeat.Start(runCtx)

	// A single bounded pool services both new deliveries and recovered PEL
	// entries, so WORKER_CONCURRENCY is an actual execution limit.
	for i := 0; i < c.cfg.Concurrency; i++ {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for {
				select {
				case msg := <-c.jobs:
					c.processAndReport(runCtx, msg)
				case <-c.stopChan:
					return
				case <-runCtx.Done():
					return
				}
			}
		}()
	}

	// Recovery routine
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.cfg.RecoveryInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.runRecoveryPass(runCtx)
			case <-c.stopChan:
				return
			case <-runCtx.Done():
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
			case <-runCtx.Done():
				return
			default:
				// Read new messages with XREADGROUP
				streams, err := c.client.XReadGroup(runCtx, &redis.XReadGroupArgs{
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
					case <-runCtx.Done():
						return
					default:
						time.Sleep(200 * time.Millisecond)
						continue
					}
				}

				for _, stream := range streams {
					for _, msg := range stream.Messages {
						if !c.submit(runCtx, msg) {
							return
						}
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
		c.metrics.IncRecovered()
		if !c.submit(ctx, msg) {
			return
		}
	}
}

func (c *Consumer) submit(ctx context.Context, msg redis.XMessage) bool {
	select {
	case c.jobs <- msg:
		return true
	case <-c.stopChan:
		return false
	case <-ctx.Done():
		return false
	}
}

func (c *Consumer) processAndReport(ctx context.Context, msg redis.XMessage) {
	if err := c.ProcessJob(ctx, msg); err != nil {
		c.metrics.IncProcessErrors()
		log.Printf("[worker %s] ProcessJob failed for message %s: %v", c.cfg.WorkerID, msg.ID, err)
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
	if c.runCancel != nil {
		c.runCancel()
	}
	c.heartbeat.Stop()
	c.wg.Wait()
}

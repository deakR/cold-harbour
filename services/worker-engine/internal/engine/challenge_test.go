package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"coldharbor/worker-engine/internal/config"
	"coldharbor/worker-engine/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// trackingPublisher records every event emitted by FSM for step audit
type trackingPublisher struct {
	mu     sync.Mutex
	events []model.EventMessage
}

func (p *trackingPublisher) Publish(ctx context.Context, event *model.EventMessage) error {
	if event == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, *event)
	return nil
}

func (p *trackingPublisher) getEvents() []model.EventMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	copied := make([]model.EventMessage, len(p.events))
	copy(copied, p.events)
	return copied
}

// 1. Empirical test for crash at 25% (step 1)
func TestEmpiricalCrashRecovery25Pct(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	scratchpad := NewScratchpadManager(rdb)
	archive := NewArchiveManager(rdb, 3600*time.Second)
	runner := NewTaskRunner(scratchpad)

	compartmentID := "cpt-crash-25pct"
	job := &model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-adversary",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize":           float64(500),
			"simulateCrashAtStep": float64(1), // Crash at step 1 (25%)
		},
	}

	// --- Phase 1: Worker 1 executes and crashes at step 1 ---
	pub1 := &trackingPublisher{}
	fsm1 := NewFSM(compartmentID, "worker-alpha", model.StateRunning, pub1)
	rec1 := NewCheckpointRecorder(scratchpad, fsm1)

	_, err := runner.Execute(ctx, job, rec1)
	if err == nil || !strings.Contains(err.Error(), "SIMULATED_WORKER_CRASH at step 1 (25%)") {
		t.Fatalf("expected crash at step 1 (25%%), got error: %v", err)
	}

	// Verify scratchpad checkpoint state after 25% crash
	step, pct, inter, err := scratchpad.GetStep(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to query scratchpad after crash: %v", err)
	}
	if step != 1 || pct != 25 {
		t.Fatalf("scratchpad mismatch after 25%% crash: step=%d (expected 1), pct=%d (expected 25)", step, pct)
	}

	var stateMap map[string]any
	if err := json.Unmarshal([]byte(inter), &stateMap); err != nil {
		t.Fatalf("failed to parse intermediate scratchpad JSON: %v", err)
	}
	if acc, ok := stateMap["accumulator"].(float64); !ok || acc != 5000 {
		t.Fatalf("scratchpad accumulator mismatch: expected 5000, got %v", acc)
	}

	// --- Phase 2: Worker 2 resumes from checkpoint ---
	// Job without crash flag
	resumedJob := &model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-adversary",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize": float64(500),
		},
	}

	pub2 := &trackingPublisher{}
	fsm2 := NewFSM(compartmentID, "worker-beta", model.StateRunning, pub2)
	rec2 := NewCheckpointRecorder(scratchpad, fsm2)

	taskRes, err := runner.Execute(ctx, resumedJob, rec2)
	if err != nil {
		t.Fatalf("worker-beta failed to resume from 25%% checkpoint: %v", err)
	}

	// Verify that Step 1 was NOT re-run by worker 2!
	// Worker 2 should only have emitted checkpoints for Step 2 (50%) and Step 3 (75%).
	events2 := pub2.getEvents()
	var checkpointPcts []int
	for _, e := range events2 {
		if e.ToState == model.StateCheckpoint {
			checkpointPcts = append(checkpointPcts, e.CheckpointPct)
		}
	}

	if len(checkpointPcts) != 2 {
		t.Fatalf("expected exactly 2 checkpoints (50%% and 75%%) during resumption, got %d: %v", len(checkpointPcts), checkpointPcts)
	}
	if checkpointPcts[0] != 50 || checkpointPcts[1] != 75 {
		t.Fatalf("checkpoint sequence mismatch on resumption: expected [50, 75], got %v", checkpointPcts)
	}

	// Verify final output correctness
	if taskRes.Output["status"] != "SUCCESS" || taskRes.Output["reducedSum"] != int64(49201) {
		t.Fatalf("final task output invalid after 25%% resumption: %+v", taskRes.Output)
	}

	// Seal archive and purge scratchpad to verify end-of-lifecycle
	dd := &model.DeadDropPayload{
		CompartmentID: compartmentID,
		OwnerID:       job.OwnerID,
		Context:       job.Context,
		TaskType:      job.TaskType,
		Output:        taskRes.Output,
		DurationMs:    taskRes.DurationMs,
		TTLSeconds:    3600,
	}
	if err := archive.Seal(ctx, dd); err != nil {
		t.Fatalf("failed to seal archive: %v", err)
	}

	valid, err := archive.VerifyChecksum(dd)
	if err != nil || !valid {
		t.Fatalf("archive checksum invalid: valid=%v, err=%v", valid, err)
	}

	if err := scratchpad.Purge(ctx, compartmentID); err != nil {
		t.Fatalf("failed to purge scratchpad: %v", err)
	}
	exists, err := scratchpad.Exists(ctx, compartmentID)
	if err != nil || exists {
		t.Fatalf("scratchpad must be non-existent post-purge")
	}
}

// 2. Empirical test for crash at 50% (step 2)
func TestEmpiricalCrashRecovery50Pct(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	scratchpad := NewScratchpadManager(rdb)
	archive := NewArchiveManager(rdb, 3600*time.Second)
	runner := NewTaskRunner(scratchpad)

	compartmentID := "cpt-crash-50pct"
	job := &model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-adversary",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize":           float64(500),
			"simulateCrashAtStep": float64(2), // Crash at step 2 (50%)
		},
	}

	// --- Phase 1: Worker 1 executes and crashes at step 2 ---
	pub1 := &trackingPublisher{}
	fsm1 := NewFSM(compartmentID, "worker-alpha", model.StateRunning, pub1)
	rec1 := NewCheckpointRecorder(scratchpad, fsm1)

	_, err := runner.Execute(ctx, job, rec1)
	if err == nil || !strings.Contains(err.Error(), "SIMULATED_WORKER_CRASH at step 2 (50%)") {
		t.Fatalf("expected crash at step 2 (50%%), got error: %v", err)
	}

	// Verify scratchpad state holds step 2 (50%)
	step, pct, inter, err := scratchpad.GetStep(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to query scratchpad: %v", err)
	}
	if step != 2 || pct != 50 {
		t.Fatalf("scratchpad mismatch after 50%% crash: step=%d, pct=%d", step, pct)
	}

	var stateMap map[string]any
	if err := json.Unmarshal([]byte(inter), &stateMap); err != nil {
		t.Fatalf("failed to unmarshal intermediate: %v", err)
	}
	// Step 1: 500*10 = 5000, Step 2: + 500*25 = 12500 -> total 17500
	if acc, ok := stateMap["accumulator"].(float64); !ok || acc != 17500 {
		t.Fatalf("intermediate accumulator mismatch: expected 17500, got %v", acc)
	}

	// --- Phase 2: Worker 2 resumes from 50% checkpoint ---
	resumedJob := &model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-adversary",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize": float64(500),
		},
	}

	pub2 := &trackingPublisher{}
	fsm2 := NewFSM(compartmentID, "worker-beta", model.StateRunning, pub2)
	rec2 := NewCheckpointRecorder(scratchpad, fsm2)

	taskRes, err := runner.Execute(ctx, resumedJob, rec2)
	if err != nil {
		t.Fatalf("resumption from 50%% failed: %v", err)
	}

	// Verify that Steps 1 and 2 were NOT re-run by worker 2!
	// Worker 2 should only emit checkpoint for Step 3 (75%).
	events2 := pub2.getEvents()
	var checkpointPcts []int
	for _, e := range events2 {
		if e.ToState == model.StateCheckpoint {
			checkpointPcts = append(checkpointPcts, e.CheckpointPct)
		}
	}

	if len(checkpointPcts) != 1 || checkpointPcts[0] != 75 {
		t.Fatalf("expected only 75%% checkpoint on resumption from 50%%, got: %v", checkpointPcts)
	}

	if taskRes.Output["reducedSum"] != int64(49201) {
		t.Fatalf("final sum mismatch: expected 49201, got %v", taskRes.Output["reducedSum"])
	}

	_ = archive.Seal(ctx, &model.DeadDropPayload{
		CompartmentID: compartmentID,
		Output:        taskRes.Output,
	})
	_ = scratchpad.Purge(ctx, compartmentID)
}

// 3. Empirical test for crash at 75% (step 3)
func TestEmpiricalCrashRecovery75Pct(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	scratchpad := NewScratchpadManager(rdb)
	archive := NewArchiveManager(rdb, 3600*time.Second)
	runner := NewTaskRunner(scratchpad)

	compartmentID := "cpt-crash-75pct"
	job := &model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-adversary",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize":           float64(500),
			"simulateCrashAtStep": float64(3), // Crash at step 3 (75%)
		},
	}

	// --- Phase 1: Worker 1 executes and crashes at step 3 ---
	pub1 := &trackingPublisher{}
	fsm1 := NewFSM(compartmentID, "worker-alpha", model.StateRunning, pub1)
	rec1 := NewCheckpointRecorder(scratchpad, fsm1)

	_, err := runner.Execute(ctx, job, rec1)
	if err == nil || !strings.Contains(err.Error(), "SIMULATED_WORKER_CRASH at step 3 (75%)") {
		t.Fatalf("expected crash at step 3 (75%%), got: %v", err)
	}

	// Verify scratchpad state holds step 3 (75%)
	step, pct, inter, err := scratchpad.GetStep(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to query scratchpad: %v", err)
	}
	if step != 3 || pct != 75 {
		t.Fatalf("scratchpad mismatch after 75%% crash: step=%d, pct=%d", step, pct)
	}

	var stateMap map[string]any
	if err := json.Unmarshal([]byte(inter), &stateMap); err != nil {
		t.Fatalf("failed to unmarshal intermediate: %v", err)
	}
	// Step 1: 5000, Step 2: 12500, Step 3: 500*42 = 21000 -> total 38500
	if acc, ok := stateMap["accumulator"].(float64); !ok || acc != 38500 {
		t.Fatalf("intermediate accumulator mismatch: expected 38500, got %v", acc)
	}

	// --- Phase 2: Worker 2 resumes from 75% checkpoint ---
	resumedJob := &model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-adversary",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize": float64(500),
		},
	}

	pub2 := &trackingPublisher{}
	fsm2 := NewFSM(compartmentID, "worker-beta", model.StateRunning, pub2)
	rec2 := NewCheckpointRecorder(scratchpad, fsm2)

	taskRes, err := runner.Execute(ctx, resumedJob, rec2)
	if err != nil {
		t.Fatalf("resumption from 75%% failed: %v", err)
	}

	// Verify that Steps 1, 2, and 3 were NOT re-run by worker 2!
	// Zero checkpoints should be emitted by worker 2 during execution of step 4!
	events2 := pub2.getEvents()
	for _, e := range events2 {
		if e.ToState == model.StateCheckpoint {
			t.Fatalf("unexpected checkpoint emitted during resumption from 75%%: %+v", e)
		}
	}

	if taskRes.Output["reducedSum"] != int64(49201) {
		t.Fatalf("final sum mismatch: expected 49201, got %v", taskRes.Output["reducedSum"])
	}

	_ = archive.Seal(ctx, &model.DeadDropPayload{
		CompartmentID: compartmentID,
		Output:        taskRes.Output,
	})
	_ = scratchpad.Purge(ctx, compartmentID)
}

// 4. Empirical test for sequential cascading crashes (25% -> 50% -> 75% -> 100%) on the SAME job
func TestEmpiricalMultiStepCascadingCrashRecovery(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	scratchpad := NewScratchpadManager(rdb)
	archive := NewArchiveManager(rdb, 3600*time.Second)
	runner := NewTaskRunner(scratchpad)

	compartmentID := "cpt-cascade-crash"

	// --- Step 1: Worker 1 runs and crashes at 25% ---
	jobStep1 := &model.JobMessage{
		CompartmentID: compartmentID,
		Payload: map[string]any{
			"batchSize":           float64(500),
			"simulateCrashAtStep": float64(1),
		},
	}
	pub1 := &trackingPublisher{}
	fsm1 := NewFSM(compartmentID, "w1", model.StateRunning, pub1)
	_, err := runner.Execute(ctx, jobStep1, NewCheckpointRecorder(scratchpad, fsm1))
	if err == nil || !strings.Contains(err.Error(), "step 1 (25%)") {
		t.Fatalf("expected step 1 crash, got: %v", err)
	}

	s1, p1, _, _ := scratchpad.GetStep(ctx, compartmentID)
	if s1 != 1 || p1 != 25 {
		t.Fatalf("expected step 1 (25%%), got step=%d, pct=%d", s1, p1)
	}

	// --- Step 2: Worker 2 resumes at 25%, runs step 2, and crashes at 50% ---
	jobStep2 := &model.JobMessage{
		CompartmentID: compartmentID,
		Payload: map[string]any{
			"batchSize":           float64(500),
			"simulateCrashAtStep": float64(2),
		},
	}
	pub2 := &trackingPublisher{}
	fsm2 := NewFSM(compartmentID, "w2", model.StateRunning, pub2)
	_, err = runner.Execute(ctx, jobStep2, NewCheckpointRecorder(scratchpad, fsm2))
	if err == nil || !strings.Contains(err.Error(), "step 2 (50%)") {
		t.Fatalf("expected step 2 crash, got: %v", err)
	}

	// Verify Worker 2 did NOT execute step 1:
	for _, e := range pub2.getEvents() {
		if e.CheckpointPct == 25 {
			t.Fatalf("Worker 2 re-executed step 1 (25%%)!")
		}
	}

	s2, p2, _, _ := scratchpad.GetStep(ctx, compartmentID)
	if s2 != 2 || p2 != 50 {
		t.Fatalf("expected step 2 (50%%), got step=%d, pct=%d", s2, p2)
	}

	// --- Step 3: Worker 3 resumes at 50%, runs step 3, and crashes at 75% ---
	jobStep3 := &model.JobMessage{
		CompartmentID: compartmentID,
		Payload: map[string]any{
			"batchSize":           float64(500),
			"simulateCrashAtStep": float64(3),
		},
	}
	pub3 := &trackingPublisher{}
	fsm3 := NewFSM(compartmentID, "w3", model.StateRunning, pub3)
	_, err = runner.Execute(ctx, jobStep3, NewCheckpointRecorder(scratchpad, fsm3))
	if err == nil || !strings.Contains(err.Error(), "step 3 (75%)") {
		t.Fatalf("expected step 3 crash, got: %v", err)
	}

	// Verify Worker 3 did NOT execute step 1 or step 2:
	for _, e := range pub3.getEvents() {
		if e.CheckpointPct == 25 || e.CheckpointPct == 50 {
			t.Fatalf("Worker 3 re-executed step 1 or 2!")
		}
	}

	s3, p3, _, _ := scratchpad.GetStep(ctx, compartmentID)
	if s3 != 3 || p3 != 75 {
		t.Fatalf("expected step 3 (75%%), got step=%d, pct=%d", s3, p3)
	}

	// --- Step 4: Worker 4 resumes at 75%, runs final step 4, and completes ---
	jobFinal := &model.JobMessage{
		CompartmentID: compartmentID,
		Payload: map[string]any{
			"batchSize": float64(500),
		},
	}
	pub4 := &trackingPublisher{}
	fsm4 := NewFSM(compartmentID, "w4", model.StateRunning, pub4)
	taskRes, err := runner.Execute(ctx, jobFinal, NewCheckpointRecorder(scratchpad, fsm4))
	if err != nil {
		t.Fatalf("Worker 4 failed to complete from 75%%: %v", err)
	}

	// Verify Worker 4 did not execute steps 1, 2, or 3
	if len(pub4.getEvents()) > 0 {
		for _, e := range pub4.getEvents() {
			if e.ToState == model.StateCheckpoint {
				t.Fatalf("Worker 4 re-executed checkpoint steps!")
			}
		}
	}

	// Final verification of arithmetic output
	if taskRes.Output["reducedSum"] != int64(49201) {
		t.Fatalf("cascaded resumption produced incorrect output sum: %v", taskRes.Output["reducedSum"])
	}

	_ = archive.Seal(ctx, &model.DeadDropPayload{
		CompartmentID: compartmentID,
		Output:        taskRes.Output,
	})
	_ = scratchpad.Purge(ctx, compartmentID)
}

// 5. Stress test: Concurrent jobs with varying simulated crash stages and concurrent resumption
func TestEmpiricalConcurrentJobRecoveryStress(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	scratchpad := NewScratchpadManager(rdb)
	archive := NewArchiveManager(rdb, 3600*time.Second)
	runner := NewTaskRunner(scratchpad)

	jobCount := 12
	type jobSpec struct {
		id          string
		crashAtStep int
	}

	specs := make([]jobSpec, jobCount)
	for i := 0; i < jobCount; i++ {
		specs[i] = jobSpec{
			id:          fmt.Sprintf("cpt-stress-%02d", i),
			crashAtStep: i % 4, // 0: no crash, 1: 25%, 2: 50%, 3: 75%
		}
	}

	// Phase 1: Run all jobs concurrently
	var wg1 sync.WaitGroup
	for _, spec := range specs {
		wg1.Add(1)
		go func(sp jobSpec) {
			defer wg1.Done()
			job := &model.JobMessage{
				CompartmentID: sp.id,
				Payload: map[string]any{
					"batchSize":           float64(500),
					"simulateCrashAtStep": float64(sp.crashAtStep),
				},
			}
			pub := &trackingPublisher{}
			fsm := NewFSM(sp.id, "worker-p1", model.StateRunning, pub)
			rec := NewCheckpointRecorder(scratchpad, fsm)

			res, err := runner.Execute(ctx, job, rec)
			if sp.crashAtStep == 0 {
				if err != nil {
					t.Errorf("job %s failed without crash: %v", sp.id, err)
					return
				}
				// Complete, seal, purge
				_ = archive.Seal(ctx, &model.DeadDropPayload{CompartmentID: sp.id, Output: res.Output})
				_ = scratchpad.Purge(ctx, sp.id)
			} else {
				if err == nil {
					t.Errorf("job %s expected crash at step %d, got nil", sp.id, sp.crashAtStep)
				}
			}
		}(spec)
	}
	wg1.Wait()

	// Verify that crashed jobs have correct checkpoints in scratchpad, and non-crashed are purged
	for _, spec := range specs {
		step, pct, _, err := scratchpad.GetStep(ctx, spec.id)
		if err != nil {
			t.Fatalf("error getting step for %s: %v", spec.id, err)
		}
		if spec.crashAtStep == 0 {
			if step != 0 {
				t.Fatalf("job %s should have 0 step (purged), got %d", spec.id, step)
			}
		} else {
			if step != spec.crashAtStep {
				t.Fatalf("job %s step mismatch: expected %d, got %d (pct=%d)", spec.id, spec.crashAtStep, step, pct)
			}
		}
	}

	// Phase 2: Concurrently resume all crashed jobs
	var wg2 sync.WaitGroup
	for _, spec := range specs {
		if spec.crashAtStep == 0 {
			continue
		}
		wg2.Add(1)
		go func(sp jobSpec) {
			defer wg2.Done()
			resumeJob := &model.JobMessage{
				CompartmentID: sp.id,
				Payload: map[string]any{
					"batchSize": float64(500),
				},
			}
			pub := &trackingPublisher{}
			fsm := NewFSM(sp.id, "worker-p2", model.StateRunning, pub)
			rec := NewCheckpointRecorder(scratchpad, fsm)

			res, err := runner.Execute(ctx, resumeJob, rec)
			if err != nil {
				t.Errorf("job %s failed to resume: %v", sp.id, err)
				return
			}

			if res.Output["reducedSum"] != int64(49201) {
				t.Errorf("job %s output invalid: %v", sp.id, res.Output["reducedSum"])
				return
			}

			if err := archive.Seal(ctx, &model.DeadDropPayload{
				CompartmentID: sp.id,
				Output:        res.Output,
			}); err != nil {
				t.Errorf("job %s archive seal failed: %v", sp.id, err)
			}

			if err := scratchpad.Purge(ctx, sp.id); err != nil {
				t.Errorf("job %s scratchpad purge failed: %v", sp.id, err)
			}
		}(spec)
	}
	wg2.Wait()

	// Invariant check: All compartments must have sealed archives and zero scratchpad leaks
	for _, spec := range specs {
		exists, err := scratchpad.Exists(ctx, spec.id)
		if err != nil || exists {
			t.Fatalf("leak detected: scratchpad for %s still exists", spec.id)
		}

		dd, err := archive.Get(ctx, spec.id)
		if err != nil || dd == nil {
			t.Fatalf("dead drop archive missing for %s", spec.id)
		}
		valid, err := archive.VerifyChecksum(dd)
		if err != nil || !valid {
			t.Fatalf("dead drop checksum invalid for %s", spec.id)
		}
	}
}

// 6. Empirical test for stream PEL reclamation and end-to-end consumer crash recovery
func TestEmpiricalConsumerStreamCrashRecoveryEndToEnd(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()

	cfg1 := config.DefaultConfig()
	cfg1.WorkerID = "worker-first-crash"
	cfg1.StreamName = "coldharbor:jobs:stream-challenge"
	cfg1.ConsumerGroup = "test-group"
	cfg1.MaxRetries = 5

	consumer1 := NewConsumer(cfg1, rdb)
	if err := consumer1.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("failed to init consumer group: %v", err)
	}

	compartmentID := "cpt-stream-crash-resumption"
	job := model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-e2e",
		TaskType:      "DATA_REDUCTION",
		MaxRetries:    5,
		Payload: map[string]any{
			"batchSize":           float64(500),
			"simulateCrashAtStep": float64(2), // 50% crash
		},
		CreatedAt: time.Now().UTC(),
	}
	jobJSON, _ := json.Marshal(job)

	msgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: cfg1.StreamName,
		Values: map[string]any{"data": string(jobJSON)},
	}).Result()
	if err != nil {
		t.Fatalf("failed to XAdd: %v", err)
	}

	// Worker 1 reads and processes message -> triggers simulated crash
	read1, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    cfg1.ConsumerGroup,
		Consumer: cfg1.WorkerID,
		Streams:  []string{cfg1.StreamName, ">"},
		Count:    1,
	}).Result()
	if err != nil || len(read1) == 0 {
		t.Fatalf("worker 1 failed to read message: %v", err)
	}

	err = consumer1.ProcessJob(ctx, read1[0].Messages[0])
	if err == nil {
		t.Fatalf("expected simulated crash on worker 1")
	}

	// Verify message remains pending in PEL (NOT acknowledged)
	pendings, err := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: cfg1.StreamName,
		Group:  cfg1.ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if err != nil || len(pendings) == 0 {
		t.Fatalf("message should be in PEL, but not found: %v", err)
	}

	// Verify scratchpad holds step 2 (50%) checkpoint
	step, pct, _, err := consumer1.scratchpad.GetStep(ctx, compartmentID)
	if err != nil || step != 2 || pct != 50 {
		t.Fatalf("scratchpad checkpoint missing: step=%d, pct=%d", step, pct)
	}

	// Now worker 2 reclaims the pending entry
	cfg2 := config.DefaultConfig()
	cfg2.WorkerID = "worker-second-recovery"
	cfg2.StreamName = cfg1.StreamName
	cfg2.ConsumerGroup = cfg1.ConsumerGroup
	cfg2.MinIdleRecoveryTime = 1 * time.Millisecond
	consumer2 := NewConsumer(cfg2, rdb)
	time.Sleep(2 * time.Millisecond)
	// Claim pending job via recovery manager
	claimedMsgs, err := consumer2.recovery.ClaimPendingJobs(ctx, 10)
	if err != nil {
		t.Fatalf("failed to claim pending jobs: %v", err)
	}

	if len(claimedMsgs) == 0 {
		t.Fatalf("failed to obtain pending message for worker 2 (claimedMsgs is empty)")
	}

	// Re-construct message without crash flag for resumption
	resumeJob := job
	resumeJob.Payload = map[string]any{"batchSize": float64(500)}
	resumeJSON, _ := json.Marshal(resumeJob)

	resumedMsg := redis.XMessage{
		ID:     claimedMsgs[0].ID,
		Values: map[string]any{"data": string(resumeJSON)},
	}

	err = consumer2.ProcessJob(ctx, resumedMsg)
	if err != nil {
		t.Fatalf("worker 2 failed to process resumed job: %v", err)
	}

	// Verify:
	// 1. Dead drop sealed
	dd, err := consumer2.archive.Get(ctx, compartmentID)
	if err != nil || dd == nil {
		t.Fatalf("dead drop archive missing: %v", err)
	}
	if dd.Output["reducedSum"] != float64(49201) {
		t.Fatalf("unexpected reducedSum: %v", dd.Output["reducedSum"])
	}

	// 2. Scratchpad purged (zero leak)
	exists, err := consumer2.scratchpad.Exists(ctx, compartmentID)
	if err != nil || exists {
		t.Fatalf("scratchpad exists after recovery completion!")
	}

	// 3. Message acknowledged and removed from PEL
	pendingsAfter, err := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: cfg1.StreamName,
		Group:  cfg1.ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if err == nil {
		for _, p := range pendingsAfter {
			if p.ID == msgID {
				t.Fatalf("message %s still in PEL after recovery completion", msgID)
			}
		}
	}
}

// 7. Empirical test: Resumption with corrupted intermediate data degrades safely
func TestEmpiricalCorruptedIntermediateDataDegradation(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	scratchpad := NewScratchpadManager(rdb)
	runner := NewTaskRunner(scratchpad)

	compartmentID := "cpt-corrupted-scratchpad"

	// Deliberately write corrupted JSON to intermediate_result
	_ = scratchpad.SetStep(ctx, compartmentID, 2, 50, "{not-valid-json!!")

	job := &model.JobMessage{
		CompartmentID: compartmentID,
		Payload: map[string]any{
			"batchSize": float64(500),
		},
	}
	pub := &trackingPublisher{}
	fsm := NewFSM(compartmentID, "worker-corrupted", model.StateRunning, pub)
	rec := NewCheckpointRecorder(scratchpad, fsm)

	// Execute should not panic; it will proceed safely
	res, err := runner.Execute(ctx, job, rec)
	if err != nil {
		t.Fatalf("runner panicked or failed on corrupted intermediate JSON: %v", err)
	}
	if res == nil || res.Output["status"] != "SUCCESS" {
		t.Fatalf("expected successful recovery even with corrupted intermediate, got: %+v", res)
	}
}

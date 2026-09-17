package engine

import (
	"context"
	"strings"
	"testing"

	"coldharbor/worker-engine/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestTaskRunnerExecutionAndResumption(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	pub := &mockPublisher{}
	scratchpad := NewScratchpadManager(rdb)
	runner := NewTaskRunner(scratchpad)

	compartmentID := "cpt-task-exec"
	job := &model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-99",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize": float64(500),
		},
	}

	// 1. Full successful run
	fsm := NewFSM(compartmentID, "worker-1", model.StateRunning, pub)
	recorder := NewCheckpointRecorder(scratchpad, fsm)

	res, err := runner.Execute(ctx, job, recorder)
	if err != nil {
		t.Fatalf("failed to execute task: %v", err)
	}

	if res.Output["status"] != "SUCCESS" || res.Output["reducedSum"] != int64(49201) {
		t.Fatalf("unexpected task output: %+v", res.Output)
	}

	// Clean up scratchpad for next test
	_ = scratchpad.Purge(ctx, compartmentID)

	// 2. Crash Simulation at step 2 (50%)
	crashJob := &model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-99",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize":           float64(500),
			"simulateCrashAtStep": float64(2),
		},
	}

	pub2 := &mockPublisher{}
	fsmCrash := NewFSM(compartmentID, "worker-1", model.StateRunning, pub2)
	recorderCrash := NewCheckpointRecorder(scratchpad, fsmCrash)

	_, err = runner.Execute(ctx, crashJob, recorderCrash)
	if err == nil || !strings.Contains(err.Error(), "SIMULATED_WORKER_CRASH") {
		t.Fatalf("expected simulated worker crash at step 2, got: %v", err)
	}

	// Verify scratchpad holds step 2 checkpoint
	step, pct, _, err := scratchpad.GetStep(ctx, compartmentID)
	if err != nil || step != 2 || pct != 50 {
		t.Fatalf("expected scratchpad to hold step 2 (50%%), got step=%d, pct=%d, err=%v", step, pct, err)
	}

	// 3. Resumption: worker 2 resumes the job without crash simulation flag
	resumeJob := &model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-99",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize": float64(500),
		},
	}

	pub3 := &mockPublisher{}
	fsmResume := NewFSM(compartmentID, "worker-2", model.StateRunning, pub3)
	recorderResume := NewCheckpointRecorder(scratchpad, fsmResume)

	resResume, err := runner.Execute(ctx, resumeJob, recorderResume)
	if err != nil {
		t.Fatalf("failed to resume and complete task: %v", err)
	}

	if resResume.Output["status"] != "SUCCESS" || resResume.Output["reducedSum"] != int64(49201) {
		t.Fatalf("resumed task output mismatch: %+v", resResume.Output)
	}

	// Verify that step 1 and 2 were skipped during resumption:
	// Only step 3 was executed and recorded! (2 events: RUNNING->CHECKPOINT, CHECKPOINT->RUNNING)
	if len(pub3.events) != 2 {
		t.Fatalf("expected only step 3 checkpoint to be recorded during resumption (2 events), got %d events", len(pub3.events))
	}
	if pub3.events[0].CheckpointPct != 75 {
		t.Fatalf("expected first event of resumed run to be 75%% checkpoint, got %d%%", pub3.events[0].CheckpointPct)
	}
}

func TestTaskRunnerDynamicBatchSizes(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	scratchpad := NewScratchpadManager(rdb)
	runner := NewTaskRunner(scratchpad)

	testCases := []int64{10, 50, 100, 250, 500, 1000, 2500}

	for _, batchSize := range testCases {
		pub := &mockPublisher{}
		compID := "cpt-batch-size"
		_ = scratchpad.Purge(ctx, compID)

		fsm := NewFSM(compID, "worker-test", model.StateRunning, pub)
		recorder := NewCheckpointRecorder(scratchpad, fsm)

		job := &model.JobMessage{
			CompartmentID: compID,
			Context:       "INNIE",
			OwnerID:       "usr-test",
			TaskType:      "DATA_REDUCTION",
			Payload: map[string]any{
				"batchSize": float64(batchSize),
			},
		}

		res, err := runner.Execute(ctx, job, recorder)
		if err != nil {
			t.Fatalf("failed for batchSize %d: %v", batchSize, err)
		}

		// Authentic formula: batchSize * (10 + 25 + 42 + 21) + 201 = batchSize * 98 + 201
		expectedSum := batchSize*98 + 201
		if res.Output["reducedSum"] != expectedSum {
			t.Fatalf("batchSize %d: expected %d, got %v", batchSize, expectedSum, res.Output["reducedSum"])
		}
		if res.Output["processedCount"] != batchSize {
			t.Fatalf("batchSize %d: expected count %d, got %v", batchSize, batchSize, res.Output["processedCount"])
		}
	}
}

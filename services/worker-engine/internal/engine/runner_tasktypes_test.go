package engine

import (
	"context"
	"testing"

	"coldharbor/worker-engine/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func executeTaskType(t *testing.T, taskType string) map[string]any {
	t.Helper()
	s := miniredis.RunT(t)
	defer s.Close()
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	scratchpad := NewScratchpadManager(rdb)
	runner := NewTaskRunner(scratchpad)
	fsm := NewFSM("cpt-"+taskType, "worker-1", model.StateRunning, &mockPublisher{})
	recorder := NewCheckpointRecorder(scratchpad, fsm)
	res, err := runner.Execute(ctx, &model.JobMessage{
		CompartmentID: "cpt-" + taskType,
		Context:       "INNIE",
		OwnerID:       "usr-1",
		TaskType:      taskType,
		Payload:       map[string]any{"batchSize": float64(100), "inputValues": []any{float64(1), float64(2)}},
	}, recorder)
	if err != nil {
		t.Fatalf("%s execution failed: %v", taskType, err)
	}
	return res.Output
}

func TestTaskRunnerDistinctTaskTypes(t *testing.T) {
	reduction := executeTaskType(t, "DATA_REDUCTION")
	if reduction["status"] != "SUCCESS" || reduction["reducedSum"] == nil {
		t.Fatalf("DATA_REDUCTION output missing reducedSum: %+v", reduction)
	}
	cipher := executeTaskType(t, "CIPHER_STREAM")
	digest, _ := cipher["streamDigest"].(string)
	if cipher["status"] != "SUCCESS" || len(digest) != 64 {
		t.Fatalf("CIPHER_STREAM output missing 64-char streamDigest: %+v", cipher)
	}
	seal := executeTaskType(t, "ARCHIVE_SEAL")
	if seal["status"] != "SUCCESS" || seal["sealedCount"] == nil || seal["sealChecksum"] == nil {
		t.Fatalf("ARCHIVE_SEAL output missing sealedCount/sealChecksum: %+v", seal)
	}
}

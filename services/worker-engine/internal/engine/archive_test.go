package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"coldharbor/worker-engine/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestEventPublisher(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	channel := "coldharbor:events"
	pub := NewEventPublisher(rdb, channel)

	sub := rdb.Subscribe(ctx, channel)
	defer sub.Close()

	// Wait for subscription to be active
	_, err := sub.Receive(ctx)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	event := &model.EventMessage{
		CompartmentID: "cpt-event-1",
		WorkerID:      "worker-go-1",
		FromState:     model.StateRunning,
		ToState:       model.StateCheckpoint,
		CheckpointPct: 50,
		Details:       "50% completed",
	}

	if err := pub.Publish(ctx, event); err != nil {
		t.Fatalf("failed to publish event: %v", err)
	}

	msg, err := sub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("failed to receive pubsub message: %v", err)
	}

	var received model.EventMessage
	if err := json.Unmarshal([]byte(msg.Payload), &received); err != nil {
		t.Fatalf("failed to unmarshal received event: %v", err)
	}

	if received.CompartmentID != "cpt-event-1" || received.CheckpointPct != 50 || received.ToState != model.StateCheckpoint {
		t.Fatalf("mismatched event received: %+v", received)
	}
	if received.EventID == "" {
		t.Fatalf("expected auto-generated event ID, got empty")
	}
}

func TestArchiveManager(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	arch := NewArchiveManager(rdb, 3600*time.Second)

	compartmentID := "cpt-dead-drop"
	output := map[string]any{
		"processedCount": float64(500),
		"reducedSum":     float64(49201),
		"status":         "SUCCESS",
	}

	dd := &model.DeadDropPayload{
		CompartmentID: compartmentID,
		OwnerID:       "usr_9918",
		Context:       "INNIE",
		TaskType:      "DATA_REDUCTION",
		Output:        output,
		TTLSeconds:    3600,
	}

	// 1. Seal Dead Drop (computes sha256 checksum)
	if err := arch.Seal(ctx, dd); err != nil {
		t.Fatalf("failed to seal dead drop: %v", err)
	}

	if dd.Checksum == "" {
		t.Fatalf("expected non-empty checksum after seal")
	}

	// 2. Retrieve Dead Drop
	retrieved, err := arch.Get(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to get dead drop: %v", err)
	}

	if retrieved.CompartmentID != compartmentID || retrieved.Checksum != dd.Checksum {
		t.Fatalf("retrieved dead drop mismatch: expected checksum %s, got %s", dd.Checksum, retrieved.Checksum)
	}

	// 3. Verify Checksum validity
	valid, err := arch.VerifyChecksum(retrieved)
	if err != nil {
		t.Fatalf("checksum verification failed with error: %v", err)
	}
	if !valid {
		t.Fatalf("expected valid checksum verification, got invalid")
	}

	// 4. Verify TTL is applied in Redis
	ttl := s.TTL(arch.Key(compartmentID))
	if ttl <= 0 {
		t.Fatalf("expected positive TTL on archive key, got %v", ttl)
	}
}

func TestArchiveManagerStringAndNonMapPayload(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	arch := NewArchiveManager(rdb, 3600*time.Second)

	// 1. String ResultPayload
	ddStr := &model.DeadDropPayload{
		CompartmentID: "cpt-string-res",
		OwnerID:       "usr_str",
		Context:       "OUTIE",
		TaskType:      "ARCHIVE_SEAL",
		ResultPayload: "sealed-string-content-12345",
		TTLSeconds:    3600,
	}

	if err := arch.Seal(ctx, ddStr); err != nil {
		t.Fatalf("failed to seal string payload: %v", err)
	}

	validStr, err := arch.VerifyChecksum(ddStr)
	if err != nil || !validStr {
		t.Fatalf("checksum verification failed for string payload: valid=%v, err=%v", validStr, err)
	}

	retrievedStr, err := arch.Get(ctx, "cpt-string-res")
	if err != nil || retrievedStr == nil {
		t.Fatalf("failed to get string dead drop: %v", err)
	}

	// 2. Byte slice ResultPayload
	ddBytes := &model.DeadDropPayload{
		CompartmentID: "cpt-bytes-res",
		OwnerID:       "usr_bytes",
		Context:       "INNIE",
		TaskType:      "CIPHER_STREAM",
		ResultPayload: []byte{0x01, 0x02, 0x03, 0x04},
		TTLSeconds:    3600,
	}

	if err := arch.Seal(ctx, ddBytes); err != nil {
		t.Fatalf("failed to seal bytes payload: %v", err)
	}

	validBytes, err := arch.VerifyChecksum(ddBytes)
	if err != nil || !validBytes {
		t.Fatalf("checksum verification failed for bytes payload: valid=%v, err=%v", validBytes, err)
	}

	// 3. Nil ResultPayload with Output nil
	ddNil := &model.DeadDropPayload{
		CompartmentID: "cpt-nil-res",
		OwnerID:       "usr_nil",
		TTLSeconds:    3600,
	}

	if err := arch.Seal(ctx, ddNil); err != nil {
		t.Fatalf("failed to seal nil payload: %v", err)
	}

	validNil, err := arch.VerifyChecksum(ddNil)
	if err != nil || !validNil {
		t.Fatalf("checksum verification failed for nil payload: valid=%v, err=%v", validNil, err)
	}
}

package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"coldharbor/worker-engine/internal/config"
	"coldharbor/worker-engine/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// 1. EMPIRICAL VERIFICATION: Zero-Leak Scratchpad Purge (EXISTS == 0) across all paths
func TestChallengeZeroLeakPurgeSuccessAndFailure(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := config.DefaultConfig()
	cfg.WorkerID = "challenger-worker-1"
	consumer := NewConsumer(cfg, rdb)
	if err := consumer.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("InitConsumerGroup failed: %v", err)
	}

	// --- SUBTEST 1A: Success Case Zero-Leak Purge ---
	t.Run("SuccessPath_ZeroLeakPurge", func(t *testing.T) {
		compID := "cpt-challenger-success-01"
		jobMsg := model.JobMessage{
			CompartmentID: compID,
			Context:       "INNIE",
			OwnerID:       "usr-adv-1",
			TaskType:      "DATA_REDUCTION",
			Payload: map[string]any{
				"batchSize": float64(500),
			},
			CreatedAt: time.Now().UTC(),
		}
		b, _ := json.Marshal(jobMsg)

		xmsgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: cfg.StreamName,
			Values: map[string]any{"data": string(b)},
		}).Result()
		if err != nil {
			t.Fatalf("XAdd failed: %v", err)
		}

		// Read and execute
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    cfg.ConsumerGroup,
			Consumer: cfg.WorkerID,
			Streams:  []string{cfg.StreamName, ">"},
			Count:    1,
		}).Result()
		if err != nil || len(streams) == 0 {
			t.Fatalf("XReadGroup failed: %v", err)
		}

		err = consumer.ProcessJob(ctx, streams[0].Messages[0])
		if err != nil {
			t.Fatalf("ProcessJob failed on success path: %v", err)
		}

		// Zero-leak check: EXISTS == 0
		exists, err := consumer.scratchpad.Exists(ctx, compID)
		if err != nil {
			t.Fatalf("scratchpad Exists check error: %v", err)
		}
		if exists {
			t.Fatalf("FAIL: compartment:%s:mem still exists post-completion", compID)
		}

		// Also check raw Redis EXISTS
		rawExists, err := rdb.Exists(ctx, fmt.Sprintf("compartment:%s:mem", compID)).Result()
		if err != nil || rawExists != 0 {
			t.Fatalf("FAIL: raw Redis exists count is %d (expected 0) for compartment:%s:mem", rawExists, compID)
		}

		// Verify message was XACKed
		pendings, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: cfg.StreamName,
			Group:  cfg.ConsumerGroup,
			Start:  "-",
			End:    "+",
			Count:  10,
		}).Result()
		for _, p := range pendings {
			if p.ID == xmsgID {
				t.Fatalf("FAIL: message %s not acknowledged from PEL", xmsgID)
			}
		}
	})

	// --- SUBTEST 1B: Terminal Failure via DLQ Zero-Leak Purge ---
	t.Run("DLQFailurePath_ZeroLeakPurge", func(t *testing.T) {
		compID := "cpt-challenger-dlq-02"

		// Pre-populate dirty scratchpad data to simulate intermediate calculation state
		_ = consumer.scratchpad.Write(ctx, compID, map[string]any{
			"leak_test_field_1": "dirty_data_alpha",
			"leak_test_field_2": "dirty_data_beta",
			"accumulated_sum":   99999,
		})
		_ = consumer.scratchpad.SetStep(ctx, compID, 2, 50, map[string]any{"inter": "step2"})

		// Confirm scratchpad exists before failure
		existsPre, _ := consumer.scratchpad.Exists(ctx, compID)
		if !existsPre {
			t.Fatalf("scratchpad should exist before DLQ trigger")
		}

		jobMsg := model.JobMessage{
			CompartmentID: compID,
			Context:       "OUTIE",
			OwnerID:       "usr-adv-2",
			TaskType:      "CIPHER_STREAM",
			MaxRetries:    1, // Max retries = 1, so 1 failure routes directly to DLQ
			Payload: map[string]any{
				"failNow": true,
			},
		}

		// Send to stream
		b, _ := json.Marshal(jobMsg)
		xmsgID, _ := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: cfg.StreamName,
			Values: map[string]any{"data": string(b)},
		}).Result()

		_, _ = rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    cfg.ConsumerGroup,
			Consumer: cfg.WorkerID,
			Streams:  []string{cfg.StreamName, ">"},
			Count:    1,
		}).Result()

		// Route to DLQ manually or via handler
		err := consumer.dlq.RouteToDLQ(ctx, cfg.StreamName, cfg.ConsumerGroup, xmsgID, cfg.WorkerID,
			&jobMsg, 1, "simulated fatal unrecoverable failure")
		if err != nil {
			t.Fatalf("RouteToDLQ failed: %v", err)
		}

		// Zero-leak check: EXISTS == 0
		existsPost, err := consumer.scratchpad.Exists(ctx, compID)
		if err != nil {
			t.Fatalf("scratchpad Exists check error: %v", err)
		}
		if existsPost {
			t.Fatalf("FAIL: compartment:%s:mem still exists after DLQ routing!", compID)
		}

		rawExists, _ := rdb.Exists(ctx, fmt.Sprintf("compartment:%s:mem", compID)).Result()
		if rawExists != 0 {
			t.Fatalf("FAIL: raw Redis exists count is %d (expected 0) for DLQ compartment:%s:mem", rawExists, compID)
		}
	})

	// --- SUBTEST 1C: Poison Pill Stream Message ---
	t.Run("PoisonPill_ZeroLeakPurge", func(t *testing.T) {
		// Insert unparseable JSON without compartmentId
		xmsgID, _ := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: cfg.StreamName,
			Values: map[string]any{"malformed": "corrupted_payload_without_compartment"},
		}).Result()

		streams, _ := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    cfg.ConsumerGroup,
			Consumer: cfg.WorkerID,
			Streams:  []string{cfg.StreamName, ">"},
			Count:    1,
		}).Result()

		if len(streams) > 0 && len(streams[0].Messages) > 0 {
			// ProcessJob should handle poison pill by routing to DLQ and acking
			_ = consumer.ProcessJob(ctx, streams[0].Messages[0])

			// Verify DLQ received the unparseable message
			dlqMsgs, _ := rdb.XRange(ctx, cfg.DLQStreamName, "-", "+").Result()
			found := false
			for _, m := range dlqMsgs {
				if m.Values["originalMsgId"] == xmsgID {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("FAIL: unparseable poison pill %s was not routed to DLQ", xmsgID)
			}

			// Verify original stream message was XACKed
			pendings, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
				Stream: cfg.StreamName,
				Group:  cfg.ConsumerGroup,
				Start:  "-",
				End:    "+",
				Count:  10,
			}).Result()
			for _, p := range pendings {
				if p.ID == xmsgID {
					t.Fatalf("FAIL: poison pill %s was not ACKed from original stream PEL", xmsgID)
				}
			}
		}
	})
}

// 2. EMPIRICAL VERIFICATION: Dead Drop SHA-256 Seal Integrity
func TestChallengeDeadDropSHA256Integrity(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	arch := NewArchiveManager(rdb, 1800*time.Second)

	// --- SUBTEST 2A: Deterministic Checksum Matching Computed Output ---
	t.Run("DeterministicSHA256_ExactMatch", func(t *testing.T) {
		compID := "cpt-adv-seal-01"
		outputData := map[string]any{
			"batchSize":      float64(500),
			"processedCount": float64(500),
			"reducedSum":     float64(49201),
			"status":         "SUCCESS",
		}

		dd := &model.DeadDropPayload{
			CompartmentID: compID,
			OwnerID:       "usr-adv-checksum",
			Context:       "INNIE",
			TaskType:      "DATA_REDUCTION",
			Output:        outputData,
			TTLSeconds:    1800,
		}

		err := arch.Seal(ctx, dd)
		if err != nil {
			t.Fatalf("arch.Seal failed: %v", err)
		}

		// Independent calculation of expected SHA-256
		outputBytes, err := json.Marshal(outputData)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		expectedSum := sha256.Sum256(outputBytes)
		expectedHex := hex.EncodeToString(expectedSum[:])

		if dd.Checksum != expectedHex {
			t.Fatalf("FAIL: Checksum mismatch! Archive had %s, independent calculation gave %s", dd.Checksum, expectedHex)
		}

		// Retrieve from Redis and verify again
		retrieved, err := arch.Get(ctx, compID)
		if err != nil {
			t.Fatalf("arch.Get failed: %v", err)
		}
		if retrieved.Checksum != expectedHex {
			t.Fatalf("FAIL: Retrieved archive checksum %s != expected %s", retrieved.Checksum, expectedHex)
		}

		valid, err := arch.VerifyChecksum(retrieved)
		if err != nil || !valid {
			t.Fatalf("FAIL: VerifyChecksum returned valid=%v, err=%v", valid, err)
		}
	})

	// --- SUBTEST 2B: Tamper Oracle Rejection ---
	t.Run("TamperDetection_Rejected", func(t *testing.T) {
		compID := "cpt-adv-seal-tamper"
		outputData := map[string]any{
			"reducedSum": float64(49201),
			"status":     "SUCCESS",
		}

		dd := &model.DeadDropPayload{
			CompartmentID: compID,
			OwnerID:       "usr-adv-tamper",
			Output:        outputData,
			TTLSeconds:    1800,
		}

		_ = arch.Seal(ctx, dd)

		// Tamper with output data
		tampered := *dd
		tamperedOutput := map[string]any{
			"reducedSum": float64(49202), // altered 49201 -> 49202
			"status":     "SUCCESS",
		}
		tampered.Output = tamperedOutput

		valid, err := arch.VerifyChecksum(&tampered)
		if err != nil {
			t.Fatalf("VerifyChecksum error: %v", err)
		}
		if valid {
			t.Fatalf("FAIL: Tampered payload was accepted as valid!")
		}
	})

	// --- SUBTEST 2C: ResultPayload Non-Map Type Handling (Robustness) ---
	t.Run("ResultPayload_TypeSafety", func(t *testing.T) {
		compID := "cpt-adv-string-payload"

		// Test whether ResultPayload with a string or nil causes panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("FAIL: Panic detected during non-map ResultPayload handling: %v", r)
			}
		}()

		ddString := &model.DeadDropPayload{
			CompartmentID: compID,
			OwnerID:       "usr-string",
			ResultPayload: "raw_string_data_payload",
			TTLSeconds:    60,
		}

		// Check if Seal handles non-map or panics
		_ = arch.Seal(ctx, ddString)
	})
}

// 3. EMPIRICAL VERIFICATION: DLQ Semantics & Stream Acknowledgment
func TestChallengeDLQRoutingSemantics(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	cfg := config.DefaultConfig()
	cfg.WorkerID = "challenger-dlq-worker"
	cfg.MaxRetries = 3

	consumer := NewConsumer(cfg, rdb)
	_ = consumer.InitConsumerGroup(ctx)

	compID := "cpt-retry-dlq-test"
	jobMsg := model.JobMessage{
		CompartmentID: compID,
		Context:       "INNIE",
		OwnerID:       "usr-retry-1",
		TaskType:      "DATA_REDUCTION",
		MaxRetries:    3,
		Payload:       map[string]any{"batchSize": float64(500)},
	}
	b, _ := json.Marshal(jobMsg)

	xmsgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: cfg.StreamName,
		Values: map[string]any{"data": string(b)},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd failed: %v", err)
	}

	// Claim message into PEL
	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    cfg.ConsumerGroup,
		Consumer: cfg.WorkerID,
		Streams:  []string{cfg.StreamName, ">"},
		Count:    1,
	}).Result()
	if err != nil || len(streams) == 0 {
		t.Fatalf("XReadGroup failed: %v", err)
	}
	msg := streams[0].Messages[0]

	// Simulate retry increments
	r1, err := consumer.dlq.IncrementRetryCount(ctx, compID)
	if err != nil || r1 != 1 {
		t.Fatalf("retry 1 mismatch: %d, %v", r1, err)
	}
	r2, err := consumer.dlq.IncrementRetryCount(ctx, compID)
	if err != nil || r2 != 2 {
		t.Fatalf("retry 2 mismatch: %d, %v", r2, err)
	}
	r3, err := consumer.dlq.IncrementRetryCount(ctx, compID)
	if err != nil || r3 != 3 {
		t.Fatalf("retry 3 mismatch: %d, %v", r3, err)
	}

	// Pre-populate scratchpad memory before DLQ routing
	_ = consumer.scratchpad.Write(ctx, compID, map[string]any{"state": "unrecoverable_error_at_step_3"})

	// Trigger DLQ routing (retries == maxRetries)
	err = consumer.dlq.RouteToDLQ(ctx, cfg.StreamName, cfg.ConsumerGroup, msg.ID, cfg.WorkerID,
		&jobMsg, r3, "exceeded max retries: simulated failure")
	if err != nil {
		t.Fatalf("RouteToDLQ failed: %v", err)
	}

	// 3.1 Verify message in DLQ stream
	dlqMsgs, err := rdb.XRange(ctx, cfg.DLQStreamName, "-", "+").Result()
	if err != nil || len(dlqMsgs) != 1 {
		t.Fatalf("FAIL: expected 1 DLQ message, got %d (err: %v)", len(dlqMsgs), err)
	}
	dlqEntry := dlqMsgs[0]
	if dlqEntry.Values["compartmentId"] != compID {
		t.Fatalf("FAIL: DLQ message compartmentId mismatch: expected %s, got %v", compID, dlqEntry.Values["compartmentId"])
	}
	if dlqEntry.Values["originalMsgId"] != xmsgID {
		t.Fatalf("FAIL: DLQ message originalMsgId mismatch: expected %s, got %v", xmsgID, dlqEntry.Values["originalMsgId"])
	}

	// 3.2 Verify message was acknowledged (XACK) from original stream PEL
	pendings, err := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: cfg.StreamName,
		Group:  cfg.ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if err != nil {
		t.Fatalf("XPendingExt failed: %v", err)
	}
	for _, p := range pendings {
		if p.ID == xmsgID {
			t.Fatalf("FAIL: Message %s is still present in PEL after DLQ routing! It was not XACKed.", xmsgID)
		}
	}

	// 3.3 Verify retry key was cleaned up
	retryExists, _ := rdb.Exists(ctx, consumer.dlq.RetryKey(compID)).Result()
	if retryExists != 0 {
		t.Fatalf("FAIL: retry key %s was not cleaned up after DLQ routing", consumer.dlq.RetryKey(compID))
	}

	// 3.4 Verify scratchpad was strictly purged
	scratchExists, _ := consumer.scratchpad.Exists(ctx, compID)
	if scratchExists {
		t.Fatalf("FAIL: scratchpad still exists after DLQ routing! ZERO-LEAK violation.")
	}
}

// 4. EMPIRICAL STRESS TEST: Concurrent mixed execution zero-leak verification
func TestChallengeConcurrentStressPurge(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg := config.DefaultConfig()
	cfg.WorkerID = "stress-worker"
	cfg.MaxRetries = 2
	consumer := NewConsumer(cfg, rdb)
	_ = consumer.InitConsumerGroup(ctx)

	const numJobs = 30
	var wg sync.WaitGroup

	type jobScenario struct {
		id       string
		taskType string
		isFail   bool
	}

	scenarios := make([]jobScenario, numJobs)
	for i := 0; i < numJobs; i++ {
		scenarios[i] = jobScenario{
			id:       fmt.Sprintf("cpt-stress-%03d", i),
			taskType: "DATA_REDUCTION",
			isFail:   (i%3 == 0), // Every 3rd job is a failure routed to DLQ
		}
	}

	// Dispatch and execute concurrently
	for _, sc := range scenarios {
		wg.Add(1)
		go func(s jobScenario) {
			defer wg.Done()

			jobMsg := model.JobMessage{
				CompartmentID: s.id,
				Context:       "INNIE",
				OwnerID:       "usr-stress",
				TaskType:      s.taskType,
				MaxRetries:    cfg.MaxRetries,
				Payload: map[string]any{
					"batchSize": float64(500),
				},
				CreatedAt: time.Now().UTC(),
			}
			b, _ := json.Marshal(jobMsg)

			xmsgID, xErr := rdb.XAdd(ctx, &redis.XAddArgs{
				Stream: cfg.StreamName,
				Values: map[string]any{"data": string(b)},
			}).Result()
			if xErr != nil {
				return
			}

			xmsg := redis.XMessage{
				ID:     xmsgID,
				Values: map[string]any{"data": string(b)},
			}

			if s.isFail {
				// Pre-populate scratchpad to test purge on failure
				_ = consumer.scratchpad.Write(ctx, s.id, map[string]any{"data": "temp_stress_junk"})
				_ = consumer.dlq.RouteToDLQ(ctx, cfg.StreamName, cfg.ConsumerGroup, xmsgID, cfg.WorkerID,
					&jobMsg, cfg.MaxRetries, "stress simulated failure")
			} else {
				_ = consumer.ProcessJob(ctx, xmsg)
			}
		}(sc)
	}

	wg.Wait()

	// Invariant check: For 100% of finished/DLQ compartments, compartment:{id}:mem MUST NOT exist
	leakCount := 0
	for _, sc := range scenarios {
		exists, err := consumer.scratchpad.Exists(ctx, sc.id)
		if err != nil || exists {
			leakCount++
			t.Errorf("FAIL: Leak detected on compartment %s: exists=%v", sc.id, exists)
		}
	}

	if leakCount > 0 {
		t.Fatalf("ZERO-LEAK VIOLATION: %d/%d compartments failed zero-leak invariant!", leakCount, numJobs)
	}

	// Double check via global keys pattern
	keys, err := rdb.Keys(ctx, "compartment:*:mem").Result()
	if err != nil {
		t.Fatalf("failed to query keys: %v", err)
	}
	if len(keys) > 0 {
		t.Fatalf("ZERO-LEAK VIOLATION: %d surviving scratchpad keys found in Redis: %v", len(keys), keys)
	}
}

package engine

import (
	"context"
	"crypto/rand"
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



// TestEdgeCaseArchivePayloadTypes verifies that archive.go safely handles:
// strings, byte slices, maps, raw JSON, slices, numbers, booleans, and nil without panicking.
func TestEdgeCaseArchivePayloadTypes(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	arch := NewArchiveManager(rdb, 3600*time.Second)

	// 1. Test matrix of various payload types for ResultPayload
	type testCase struct {
		name    string
		payload any
	}

	largeString := strings.Repeat("A-COMPLEX-STRING-REPRESENTING-DATA-REDUCTION-OUTPUT-", 1000)
	randomBytes := make([]byte, 1024)
	_, _ = rand.Read(randomBytes)

	testCases := []testCase{
		{name: "EmptyString", payload: ""},
		{name: "SimpleString", payload: "data_reduction_output_string_hash"},
		{name: "UnicodeString", payload: "こんにちは世界 🌍 !@#$%^&*()_+{}[]|\"<>,.?/"},
		{name: "JSONString", payload: `{"batchSize":500,"reducedSum":49201,"status":"SUCCESS"}`},
		{name: "LargeString100KB", payload: largeString},

		{name: "EmptyByteSlice", payload: []byte{}},
		{name: "BinaryByteSlice", payload: []byte{0x00, 0x01, 0x02, 0xFE, 0xFF, 0x7F, 0x80}},
		{name: "Random1KBBytes", payload: randomBytes},

		{name: "EmptyMap", payload: map[string]any{}},
		{name: "SimpleMap", payload: map[string]any{"reducedSum": float64(49201), "status": "SUCCESS"}},
		{name: "DeepNestedMap", payload: map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"values": []any{1, "two", true, nil, 3.14159},
				},
			},
		}},

		{name: "RawJSONMessage", payload: json.RawMessage(`{"raw":"json","elements":[10,20,30]}`)},
		{name: "SliceOfStrings", payload: []string{"first", "second", "third"}},
		{name: "SliceOfInts", payload: []int{10, 25, 42, 99}},
		{name: "SliceOfMaps", payload: []map[string]any{{"a": 1}, {"b": "two"}}},

		{name: "Integer", payload: 49201},
		{name: "Int64", payload: int64(9876543210123)},
		{name: "Float64", payload: 3.141592653589793},
		{name: "BoolTrue", payload: true},
		{name: "BoolFalse", payload: false},

		{name: "NilPayload", payload: nil},
	}

	for _, tc := range testCases {
		t.Run("Payload_"+tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC on payload %s: %v", tc.name, r)
				}
			}()

			compID := fmt.Sprintf("cpt-test-%s", strings.ToLower(tc.name))
			dd := &model.DeadDropPayload{
				CompartmentID: compID,
				OwnerID:       "usr-challenger",
				Context:       "INNIE",
				TaskType:      "DATA_REDUCTION",
				ResultPayload: tc.payload,
				TTLSeconds:    1800,
			}

			cBefore, _ := ComputeChecksum(tc.payload)
			t.Logf("BEFORE SEAL: tc.name=%s, tc.payload=%+v, cBefore=%s", tc.name, tc.payload, cBefore)

			// 1. Seal Dead Drop
			err := arch.Seal(ctx, dd)
			if err != nil {
				t.Fatalf("Seal failed for %s: %v", tc.name, err)
			}

			t.Logf("AFTER SEAL: tc.name=%s, dd.Checksum=%s, expected cBefore=%s", tc.name, dd.Checksum, cBefore)
			if dd.Checksum != cBefore {
				t.Fatalf("CRITICAL INTEGRITY DEFECT: dd.Checksum (%s) does not match actual payload checksum (%s)! It was hashed as null (%s)",
					dd.Checksum, cBefore, "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b")
			}

			if dd.Checksum == "" {
				t.Fatalf("Seal produced empty checksum for %s", tc.name)
			}

			// 2. VerifyChecksum pre-storage
			valid, err := arch.VerifyChecksum(dd)
			if err != nil {
				t.Fatalf("VerifyChecksum returned error for %s: %v", tc.name, err)
			}
			bOutput, _ := json.Marshal(dd.Output)
			bRes, _ := json.Marshal(dd.ResultPayload)
			t.Logf("tc.name=%s, bOutput=%s, bRes=%s", tc.name, string(bOutput), string(bRes))
			if !valid {
				expectedCheck, _ := ComputeChecksum(dd.Output)
				expectedRes, _ := ComputeChecksum(dd.ResultPayload)
				t.Fatalf("VerifyChecksum returned false for %s: dd.Checksum=%s, expectedCheck=%s, expectedRes=%s, dd.Output=%+v, dd.ResultPayload=%+v",
					tc.name, dd.Checksum, expectedCheck, expectedRes, dd.Output, dd.ResultPayload)
			}

			// 3. Get from Redis
			retrieved, err := arch.Get(ctx, compID)
			if err != nil {
				t.Fatalf("Get failed for %s: %v", tc.name, err)
			}
			if retrieved.CompartmentID != compID {
				t.Fatalf("retrieved compartment ID mismatch: expected %s, got %s", compID, retrieved.CompartmentID)
			}
			if retrieved.Checksum != dd.Checksum {
				t.Fatalf("retrieved checksum mismatch for %s: expected %s, got %s", tc.name, dd.Checksum, retrieved.Checksum)
			}

			// 4. VerifyChecksum on retrieved payload (safely without panicking)
			// Note: For []byte, JSON unmarshals into string (base64) so checksum verification
			// tests the safety of handling without panic.
			_, _ = arch.VerifyChecksum(retrieved)
		})
	}

	// 2. Negative & Edge Case Tests
	t.Run("Negative_NilDeadDrop", func(t *testing.T) {
		err := arch.Seal(ctx, nil)
		if err == nil {
			t.Fatalf("expected error when sealing nil dead drop, got nil")
		}
	})

	t.Run("Negative_EmptyCompartmentID", func(t *testing.T) {
		err := arch.Seal(ctx, &model.DeadDropPayload{CompartmentID: ""})
		if err == nil {
			t.Fatalf("expected error when sealing dead drop with empty compartment ID, got nil")
		}
	})

	t.Run("Negative_VerifyChecksumNil", func(t *testing.T) {
		valid, err := arch.VerifyChecksum(nil)
		if err == nil || valid {
			t.Fatalf("expected error when verifying nil dead drop, got valid=%v, err=%v", valid, err)
		}
	})

	t.Run("Negative_UnserializablePayloadNoPanic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC on unserializable payload: %v", r)
			}
		}()

		// Channel cannot be JSON marshaled
		unsupported := make(chan int)
		_, err := ComputeChecksum(unsupported)
		if err == nil {
			t.Fatalf("expected error on unsupported channel type, got nil")
		}

		dd := &model.DeadDropPayload{
			CompartmentID: "cpt-unsupported",
			ResultPayload: unsupported,
		}
		err = arch.Seal(ctx, dd)
		if err == nil {
			t.Fatalf("expected seal error on unsupported payload, got nil")
		}
	})
}

// TestEdgeCaseZeroLeakInvariant verifies that compartment:{id}:mem strictly has
// EXISTS == 0 on:
// 1. Successful completion.
// 2. Terminal failure / DLQ after retry exhaustion.
// 3. Poison pill / unparseable message DLQ.
// 4. Recovery after crash resumption.
// 5. High-concurrency stress test with mixed outcomes.
func TestEdgeCaseZeroLeakInvariant(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg := config.DefaultConfig()
	cfg.WorkerID = "challenger-zeroleak-worker"
	cfg.MaxRetries = 2
	consumer := NewConsumer(cfg, rdb)
	if err := consumer.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("InitConsumerGroup failed: %v", err)
	}

	// 1. Success path: Scratchpad created with checkpoint data, strictly 0 post completion
	t.Run("SuccessPath_StrictZeroLeak", func(t *testing.T) {
		compID := "cpt-zl-success-verify"
		jobMsg := model.JobMessage{
			CompartmentID: compID,
			Context:       "INNIE",
			OwnerID:       "usr-zl-1",
			TaskType:      "DATA_REDUCTION",
			Payload: map[string]any{
				"batchSize": float64(500),
			},
			CreatedAt: time.Now().UTC(),
		}
		b, _ := json.Marshal(jobMsg)

		msgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: cfg.StreamName,
			Values: map[string]any{"data": string(b)},
		}).Result()
		if err != nil {
			t.Fatalf("XAdd failed: %v", err)
		}

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
			t.Fatalf("ProcessJob failed: %v", err)
		}

		// Check manager API
		exists, err := consumer.scratchpad.Exists(ctx, compID)
		if err != nil {
			t.Fatalf("scratchpad.Exists returned error: %v", err)
		}
		if exists {
			t.Fatalf("ZERO-LEAK VIOLATION: scratchpad.Exists returned true for %s", compID)
		}

		// Direct Redis check
		rawKey := fmt.Sprintf("compartment:%s:mem", compID)
		rawCount, err := rdb.Exists(ctx, rawKey).Result()
		if err != nil {
			t.Fatalf("Redis EXISTS error: %v", err)
		}
		if rawCount != 0 {
			t.Fatalf("ZERO-LEAK VIOLATION: Redis EXISTS %s returned %d (expected 0)", rawKey, rawCount)
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
			if p.ID == msgID {
				t.Fatalf("message %s still present in PEL; not XACKed", msgID)
			}
		}
	})

	// 2. Terminal Failure / DLQ across intermediate checkpoint stages (25%, 50%, 75%)
	checkpointStages := []struct {
		name string
		step int
		pct  int
	}{
		{name: "AfterStep1_25Pct", step: 1, pct: 25},
		{name: "AfterStep2_50Pct", step: 2, pct: 50},
		{name: "AfterStep3_75Pct", step: 3, pct: 75},
	}

	for _, cs := range checkpointStages {
		t.Run("TerminalFailure_DLQ_"+cs.name, func(t *testing.T) {
			compID := fmt.Sprintf("cpt-zl-fail-%s", strings.ToLower(cs.name))

			// Write dirty intermediate scratchpad state simulating mid-execution
			_ = consumer.scratchpad.SetStep(ctx, compID, cs.step, cs.pct, map[string]any{
				"intermediate_accumulator": 12345 * cs.step,
				"stage":                    cs.name,
			})
			_ = consumer.scratchpad.Write(ctx, compID, map[string]any{
				"scratchpad_dirty_buffer": "buffered_data_to_be_purged",
			})

			// Pre-check: scratchpad definitely exists
			preExists, _ := consumer.scratchpad.Exists(ctx, compID)
			if !preExists {
				t.Fatalf("precondition failed: scratchpad must exist before failure")
			}

			jobMsg := model.JobMessage{
				CompartmentID: compID,
				Context:       "OUTIE",
				OwnerID:       "usr-fail-1",
				TaskType:      "DATA_REDUCTION",
				MaxRetries:    1, // Max retries 1 means 1 failure routes straight to DLQ
				Payload:       map[string]any{"batchSize": float64(500)},
			}
			b, _ := json.Marshal(jobMsg)

			msgID, _ := rdb.XAdd(ctx, &redis.XAddArgs{
				Stream: cfg.StreamName,
				Values: map[string]any{"data": string(b)},
			}).Result()

			streams, _ := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    cfg.ConsumerGroup,
				Consumer: cfg.WorkerID,
				Streams:  []string{cfg.StreamName, ">"},
				Count:    1,
			}).Result()

			// Route directly to DLQ simulating terminal failure at this step
			err := consumer.dlq.RouteToDLQ(ctx, cfg.StreamName, cfg.ConsumerGroup, streams[0].Messages[0].ID, cfg.WorkerID,
				&jobMsg, 1, fmt.Sprintf("simulated terminal failure at %s", cs.name))
			if err != nil {
				t.Fatalf("RouteToDLQ failed: %v", err)
			}

			// Post-failure ZERO-LEAK check: EXISTS == 0
			exists, err := consumer.scratchpad.Exists(ctx, compID)
			if err != nil {
				t.Fatalf("scratchpad.Exists error: %v", err)
			}
			if exists {
				t.Fatalf("ZERO-LEAK VIOLATION: scratchpad exists after DLQ routing for %s", compID)
			}

			rawKey := fmt.Sprintf("compartment:%s:mem", compID)
			rawCount, err := rdb.Exists(ctx, rawKey).Result()
			if err != nil || rawCount != 0 {
				t.Fatalf("ZERO-LEAK VIOLATION: Redis EXISTS %s returned %d (expected 0)", rawKey, rawCount)
			}

			// Message must be acknowledged
			pendings, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
				Stream: cfg.StreamName,
				Group:  cfg.ConsumerGroup,
				Start:  "-",
				End:    "+",
				Count:  10,
			}).Result()
			for _, p := range pendings {
				if p.ID == msgID {
					t.Fatalf("message %s still present in PEL after DLQ routing", msgID)
				}
			}
		})
	}

	// 3. Poison pill: Unparseable message
	t.Run("PoisonPill_DLQ_ZeroLeak", func(t *testing.T) {
		msgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: cfg.StreamName,
			Values: map[string]any{"invalid_payload": "corrupted_non_json"},
		}).Result()
		if err != nil {
			t.Fatalf("XAdd failed: %v", err)
		}

		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    cfg.ConsumerGroup,
			Consumer: cfg.WorkerID,
			Streams:  []string{cfg.StreamName, ">"},
			Count:    1,
		}).Result()
		if err != nil || len(streams) == 0 {
			t.Fatalf("XReadGroup failed: %v", err)
		}

		// Process unparseable message -> should route to DLQ
		_ = consumer.ProcessJob(ctx, streams[0].Messages[0])

		// Verify no scratchpad leaked for unparseable ID
		unparseableCompID := fmt.Sprintf("unparseable-%s", msgID)
		exists, _ := consumer.scratchpad.Exists(ctx, unparseableCompID)
		if exists {
			t.Fatalf("ZERO-LEAK VIOLATION: scratchpad exists for %s", unparseableCompID)
		}

		// Original message acknowledged
		pendings, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: cfg.StreamName,
			Group:  cfg.ConsumerGroup,
			Start:  "-",
			End:    "+",
			Count:  10,
		}).Result()
		for _, p := range pendings {
			if p.ID == msgID {
				t.Fatalf("poison pill message %s still present in PEL", msgID)
			}
		}
	})

	// 4. Crash Recovery Resumption Zero-Leak Check
	t.Run("CrashResumption_ZeroLeak", func(t *testing.T) {
		compID := "cpt-zl-crash-resumption"
		jobMsg := model.JobMessage{
			CompartmentID: compID,
			Context:       "INNIE",
			OwnerID:       "usr-crash-zl",
			TaskType:      "DATA_REDUCTION",
			Payload: map[string]any{
				"batchSize":           float64(500),
				"simulateCrashAtStep": float64(2), // crashes at step 2 (50%)
			},
			CreatedAt: time.Now().UTC(),
		}
		b, _ := json.Marshal(jobMsg)

		msgID, _ := rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: cfg.StreamName,
			Values: map[string]any{"data": string(b)},
		}).Result()

		// Worker 1 executes and crashes at step 2
		streams, _ := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    cfg.ConsumerGroup,
			Consumer: "worker-pre-crash",
			Streams:  []string{cfg.StreamName, ">"},
			Count:    1,
		}).Result()

		worker1 := NewConsumer(cfg, rdb)
		worker1.cfg.WorkerID = "worker-pre-crash"
		crashErr := worker1.ProcessJob(ctx, streams[0].Messages[0])
		if crashErr == nil || !strings.Contains(crashErr.Error(), "SIMULATED_WORKER_CRASH") {
			t.Fatalf("expected SIMULATED_WORKER_CRASH, got %v", crashErr)
		}

		// Scratchpad should exist at 50%
		existsMid, _ := consumer.scratchpad.Exists(ctx, compID)
		if !existsMid {
			t.Fatalf("scratchpad should exist during mid-crash checkpoint")
		}

		// Worker 2 recovers job: clear crash flag to allow completion
		delete(jobMsg.Payload, "simulateCrashAtStep")
		bRecovered, _ := json.Marshal(jobMsg)

		worker2 := NewConsumer(cfg, rdb)
		worker2.cfg.WorkerID = "worker-post-recovery"

		recoveredXMsg := redis.XMessage{
			ID:     msgID,
			Values: map[string]any{"data": string(bRecovered)},
		}

		err := worker2.ProcessJob(ctx, recoveredXMsg)
		if err != nil {
			t.Fatalf("Worker 2 recovery execution failed: %v", err)
		}

		// Post-recovery: ZERO-LEAK check
		existsPost, err := worker2.scratchpad.Exists(ctx, compID)
		if err != nil || existsPost {
			t.Fatalf("ZERO-LEAK VIOLATION: scratchpad exists post-recovery for %s", compID)
		}

		rawCount, _ := rdb.Exists(ctx, fmt.Sprintf("compartment:%s:mem", compID)).Result()
		if rawCount != 0 {
			t.Fatalf("ZERO-LEAK VIOLATION: raw count %d != 0 post-recovery", rawCount)
		}
	})

	// 5. High-concurrency mixed outcome stress test (50 jobs)
	t.Run("HighConcurrencyMixed_StrictZeroLeak", func(t *testing.T) {
		const totalJobs = 50
		var wg sync.WaitGroup

		for i := 0; i < totalJobs; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				cID := fmt.Sprintf("cpt-conc-zl-%03d", idx)
				isTerminalFail := (idx%4 == 0) // 25% fail terminally to DLQ

				jMsg := model.JobMessage{
					CompartmentID: cID,
					Context:       "INNIE",
					OwnerID:       "usr-stress",
					TaskType:      "DATA_REDUCTION",
					MaxRetries:    1,
					Payload: map[string]any{
						"batchSize": float64(500),
					},
					CreatedAt: time.Now().UTC(),
				}
				data, _ := json.Marshal(jMsg)

				xID, xErr := rdb.XAdd(ctx, &redis.XAddArgs{
					Stream: cfg.StreamName,
					Values: map[string]any{"data": string(data)},
				}).Result()
				if xErr != nil {
					return
				}

				xMsg := redis.XMessage{
					ID:     xID,
					Values: map[string]any{"data": string(data)},
				}

				if isTerminalFail {
					// Pre-populate dirty scratchpad
					_ = consumer.scratchpad.SetStep(ctx, cID, 1, 25, map[string]any{"inter": "val"})
					_ = consumer.dlq.RouteToDLQ(ctx, cfg.StreamName, cfg.ConsumerGroup, xID, cfg.WorkerID,
						&jMsg, 1, "stress failure")
				} else {
					_ = consumer.ProcessJob(ctx, xMsg)
				}
			}(i)
		}

		wg.Wait()

		// Strict Invariant verification across ALL 50 compartments
		for i := 0; i < totalJobs; i++ {
			cID := fmt.Sprintf("cpt-conc-zl-%03d", i)
			ex, err := consumer.scratchpad.Exists(ctx, cID)
			if err != nil || ex {
				t.Fatalf("ZERO-LEAK VIOLATION: compartment %s still exists: %v", cID, ex)
			}
		}

		// Global Redis check
		surviving, err := rdb.Keys(ctx, "compartment:*:mem").Result()
		if err != nil {
			t.Fatalf("Keys query error: %v", err)
		}
		if len(surviving) > 0 {
			t.Fatalf("ZERO-LEAK VIOLATION: %d orphan scratchpad keys found: %v", len(surviving), surviving)
		}
	})
}

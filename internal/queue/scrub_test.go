package queue

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestScrubDeletesSettledInputAndKeepsLiveJobs(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()

	settled, err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{"id": "old", "input": "secret-old", "tenant_id": "t1"},
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{"id": "clean", "tenant_id": "t1"},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	pendingID, err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{"id": "inflight", "input": "secret-inflight"},
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	read, err := stream.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    workerGroup,
		Consumer: "scrub-test",
		Streams:  []string{jobsStreamKey, ">"},
		Count:    1,
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 1 || len(read[0].Messages) != 1 || read[0].Messages[0].ID != pendingID {
		t.Fatalf("read = %+v, want pending %s", read, pendingID)
	}
	queuedID, err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{"id": "queued", "input": "secret-queued"},
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	dlqID, err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStreamKey,
		Values: map[string]any{"id": "buried", "input": "secret-dlq", "tenant_id": "t1"},
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	bareID, err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStreamKey,
		Values: map[string]any{"input": "secret-only"},
	}).Result()
	if err != nil {
		t.Fatal(err)
	}

	report, err := stream.ScrubPlaintext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.MainDeleted) != 1 || report.MainDeleted[0] != settled {
		t.Fatalf("MainDeleted = %v, want [%s]", report.MainDeleted, settled)
	}
	if len(report.MainKept) != 2 {
		t.Fatalf("MainKept = %+v, want 2", report.MainKept)
	}
	kept := map[string]string{}
	for _, row := range report.MainKept {
		kept[row.ID] = row.Reason
	}
	if kept[pendingID] != "pending" {
		t.Fatalf("pending reason = %q, want pending", kept[pendingID])
	}
	if kept[queuedID] != "not yet delivered" {
		t.Fatalf("queued reason = %q, want not yet delivered", kept[queuedID])
	}
	if len(report.DLQRewritten) != 1 || report.DLQRewritten[0] != dlqID {
		t.Fatalf("DLQRewritten = %v, want [%s]", report.DLQRewritten, dlqID)
	}
	if len(report.DLQRemoved) != 1 || report.DLQRemoved[0] != bareID {
		t.Fatalf("DLQRemoved = %v, want [%s]", report.DLQRemoved, bareID)
	}

	main, err := stream.rdb.XRange(ctx, jobsStreamKey, "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(main) != 3 {
		t.Fatalf("main len = %d, want 3", len(main))
	}
	for _, entry := range main {
		fields := valuesToFields(entry.Values)
		if fields["id"] == "old" {
			t.Fatal("settled entry still on the main stream")
		}
		if fields["id"] == "clean" {
			if _, ok := fields["input"]; ok {
				t.Fatal("clean entry gained an input field")
			}
			continue
		}
		if _, ok := fields["input"]; !ok {
			t.Fatalf("live entry %s lost input", fields["id"])
		}
	}

	dlq, err := stream.rdb.XRange(ctx, dlqStreamKey, "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(dlq) != 1 {
		t.Fatalf("dlq len = %d, want 1", len(dlq))
	}
	fields := valuesToFields(dlq[0].Values)
	if _, ok := fields["input"]; ok {
		t.Fatal("dlq entry still has input")
	}
	if fields["id"] != "buried" || fields["tenant_id"] != "t1" {
		t.Fatalf("dlq fields = %v", fields)
	}

	again, err := stream.ScrubPlaintext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.MainDeleted) != 0 || len(again.DLQRewritten) != 0 || len(again.DLQRemoved) != 0 {
		t.Fatalf("second scrub = %+v, want no deletions", again)
	}
	if len(again.MainKept) != 2 {
		t.Fatalf("second scrub kept = %+v, want the two live jobs", again.MainKept)
	}
}

func TestScrubWithoutGroupLeavesTheQueue(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	id, err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{"id": "waiting", "input": "secret"},
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	report, err := stream.ScrubPlaintext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.MainDeleted) != 0 {
		t.Fatalf("MainDeleted = %v, want none", report.MainDeleted)
	}
	if len(report.MainKept) != 1 || report.MainKept[0].ID != id || report.MainKept[0].Reason != "no worker-group" {
		t.Fatalf("MainKept = %+v", report.MainKept)
	}
	n, err := stream.Len(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("XLEN = %d, want 1", n)
	}
}

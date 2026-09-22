package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"coldharbour/internal/checkpoint"
	"coldharbour/internal/events"
	"coldharbour/internal/journal"
	"coldharbour/internal/redact"

	"github.com/redis/go-redis/v9"
)

const workerGroup = "worker-group"

type deliveryState int

const (
	deliveryNew deliveryState = iota
	deliveryStep1Saved
	deliveryDone
)

type claimed struct {
	entry  StreamID
	job    Job
	crash  checkpoint.CrashPoint
	fail   bool
	fields map[string]string
}

func parseClaim(entry StreamID, fields map[string]string) (claimed, error) {
	job, err := parseJob(entry, fields)
	if err != nil {
		return claimed{}, err
	}
	crash := checkpoint.CrashNever
	if fields["simulateCrashAtStep"] == "1" {
		crash = checkpoint.CrashAfterStep1
	}
	return claimed{
		entry:  entry,
		job:    job,
		crash:  crash,
		fail:   fields["simulateFailure"] == "1",
		fields: fields,
	}, nil
}

func deliveryOf(cp *checkpoint.Checkpoint) (deliveryState, error) {
	if cp == nil {
		return deliveryNew, nil
	}
	switch cp.Step {
	case 0:
		return deliveryNew, nil
	case checkpoint.StepEmailsAndPhones:
		return deliveryStep1Saved, nil
	case checkpoint.StepSSNs:
		return deliveryDone, nil
	default:
		return 0, fmt.Errorf("unknown checkpoint step %d", cp.Step)
	}
}

func (s *jobStream) ensureGroup(ctx context.Context) error {
	err := s.rdb.XGroupCreateMkStream(ctx, jobsStreamKey, workerGroup, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (s *jobStream) enqueue(ctx context.Context, job Job, crash checkpoint.CrashPoint) error {
	values := map[string]any{"input": job.Input}
	if job.ID != "" {
		values["id"] = job.ID
	}
	if job.TenantID != "" {
		values["tenant_id"] = job.TenantID
	}
	if crash == checkpoint.CrashAfterStep1 {
		values["simulateCrashAtStep"] = "1"
	}
	return s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: values,
	}).Err()
}

func (s *jobStream) ack(ctx context.Context, id StreamID) error {
	return s.rdb.XAck(ctx, jobsStreamKey, workerGroup, id.String()).Err()
}

func PrepareGroup(ctx context.Context, stream *jobStream, jobs []Job) error {
	if err := stream.ensureGroup(ctx); err != nil {
		return err
	}
	return seedIfEmpty(ctx, stream, jobs)
}

func RunGroup(ctx context.Context, stream *jobStream, cfg WorkerConfig, store journal.Journal, emit func(journal.JobResult)) error {
	if store == nil {
		panic("nil journal")
	}
	if err := cfg.valid(); err != nil {
		return err
	}
	hash := &memHash{rdb: stream.rdb}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		stolen, _, err := stream.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   jobsStreamKey,
			Group:    workerGroup,
			Consumer: cfg.Consumer,
			MinIdle:  cfg.Idle,
			Start:    "0-0",
		}).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			if err := ctx.Err(); err != nil {
				return err
			}
			return err
		}
		for _, msg := range stolen {
			if err := dispatch(ctx, stream, hash, msg, store, emit); err != nil {
				return err
			}
		}
		streams, err := stream.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    workerGroup,
			Consumer: cfg.Consumer,
			Streams:  []string{jobsStreamKey, ">"},
			Block:    cfg.Poll,
		}).Result()
		if err != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			if errors.Is(err, redis.Nil) || isReadTimeout(err) {
				continue
			}
			return err
		}
		for _, xs := range streams {
			for _, msg := range xs.Messages {
				if err := dispatch(ctx, stream, hash, msg, store, emit); err != nil {
					return err
				}
			}
		}
	}
}

func isReadTimeout(err error) bool {
	var to interface{ Timeout() bool }
	return errors.As(err, &to) && to.Timeout()
}

func dispatch(ctx context.Context, stream *jobStream, hash *memHash, msg redis.XMessage, store journal.Journal, emit func(journal.JobResult)) error {
	id, err := ParseStreamID(msg.ID)
	if err != nil {
		return err
	}
	c, err := parseClaim(id, valuesToFields(msg.Values))
	if errors.Is(err, errMissingInput) {
		return stream.ack(context.WithoutCancel(ctx), id)
	}
	if err != nil {
		return err
	}
	return handle(ctx, stream, hash, c, store, emit)
}

func handle(ctx context.Context, stream *jobStream, hash *memHash, c claimed, store journal.Journal, emit func(journal.JobResult)) error {
	cp, err := hash.load(ctx, c.job.ID)
	if err != nil {
		return err
	}
	state, err := deliveryOf(cp)
	if err != nil {
		return err
	}

	if state == deliveryNew {
		partial := redact.ApplyClassWindow(redact.RedactResult{RedactedText: c.job.Input}, 0)
		next := checkpoint.Checkpoint{JobID: c.job.ID, Step: checkpoint.StepEmailsAndPhones, PartialResult: partial}
		if err := hash.saveStep1(ctx, next); err != nil {
			return err
		}
		fmt.Println("checkpoint", journal.FormatJobLine(journal.JobResult{ID: c.job.ID, Result: partial}))
		if err := events.Publish(ctx, stream.rdb, events.JobEvent{
			TenantID: c.job.TenantID,
			JobID:    c.job.ID,
			Status:   string(journal.RUNNING),
		}); err != nil {
			return err
		}
		if c.crash == checkpoint.CrashAfterStep1 {
			return checkpoint.ErrSimulatedCrash
		}
		cp = &next
		state = deliveryStep1Saved
	}

	if state == deliveryStep1Saved {
		done := redact.ApplyClassWindow(cp.PartialResult, 1)
		next := checkpoint.Checkpoint{JobID: c.job.ID, Step: checkpoint.StepSSNs, PartialResult: done}
		if err := hash.save(ctx, next); err != nil {
			return err
		}
		cp = &next
		state = deliveryDone
	}

	if c.fail {
		result := journal.Fail(c.job.ID, cp.PartialResult)
		result.TenantID = c.job.TenantID
		return (&deadLetters{rdb: stream.rdb}).fail(context.WithoutCancel(ctx), c, result, store)
	}
	result := journal.Succeed(c.job.ID, cp.PartialResult)
	result.TenantID = c.job.TenantID
	if err := store.Record(ctx, result); err != nil {
		return err
	}
	if err := events.Publish(ctx, stream.rdb, events.JobEvent{
		TenantID: result.TenantID,
		JobID:    result.ID,
		Status:   string(journal.COMPLETED),
	}); err != nil {
		return err
	}
	emit(result)
	return stream.ack(context.WithoutCancel(ctx), c.entry)
}

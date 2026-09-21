package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	entry StreamID
	job   Job
	crash CrashPoint
}

func parseClaim(entry StreamID, fields map[string]string) (claimed, error) {
	job, err := parseJob(entry, fields)
	if err != nil {
		return claimed{}, err
	}
	crash := CrashNever
	if fields["simulateCrashAtStep"] == "1" {
		crash = CrashAfterStep1
	}
	return claimed{entry: entry, job: job, crash: crash}, nil
}

func deliveryOf(cp *Checkpoint) (deliveryState, error) {
	if cp == nil {
		return deliveryNew, nil
	}
	switch cp.Step {
	case 0:
		return deliveryNew, nil
	case StepEmailsAndPhones:
		return deliveryStep1Saved, nil
	case StepSSNs:
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

func (s *jobStream) enqueue(ctx context.Context, job Job, crash CrashPoint) error {
	values := map[string]any{"input": job.Input}
	if job.ID != "" {
		values["id"] = job.ID
	}
	if crash == CrashAfterStep1 {
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

func prepareGroup(ctx context.Context, stream *jobStream, jobs []Job) error {
	if err := stream.ensureGroup(ctx); err != nil {
		return err
	}
	return seedIfEmpty(ctx, stream, jobs)
}

func runGroup(ctx context.Context, stream *jobStream, cfg WorkerConfig, journal Journal, emit func(JobResult)) error {
	if journal == nil {
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
			if err := dispatch(ctx, stream, hash, msg, journal, emit); err != nil {
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
				if err := dispatch(ctx, stream, hash, msg, journal, emit); err != nil {
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

func dispatch(ctx context.Context, stream *jobStream, hash *memHash, msg redis.XMessage, journal Journal, emit func(JobResult)) error {
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
	return handle(ctx, stream, hash, c, journal, emit)
}

func handle(ctx context.Context, stream *jobStream, hash *memHash, c claimed, journal Journal, emit func(JobResult)) error {
	cp, err := hash.load(ctx, c.job.ID)
	if err != nil {
		return err
	}
	state, err := deliveryOf(cp)
	if err != nil {
		return err
	}

	if state == deliveryNew {
		partial := applyClassWindow(RedactResult{RedactedText: c.job.Input}, stepWindows[0])
		next := Checkpoint{JobID: c.job.ID, Step: StepEmailsAndPhones, PartialResult: partial}
		if err := hash.saveStep1(ctx, next); err != nil {
			return err
		}
		fmt.Println("checkpoint", formatJobLine(JobResult{ID: c.job.ID, Result: partial}))
		if c.crash == CrashAfterStep1 {
			return ErrSimulatedCrash
		}
		cp = &next
		state = deliveryStep1Saved
	}

	if state == deliveryStep1Saved {
		done := applyClassWindow(cp.PartialResult, stepWindows[1])
		next := Checkpoint{JobID: c.job.ID, Step: StepSSNs, PartialResult: done}
		if err := hash.save(ctx, next); err != nil {
			return err
		}
		cp = &next
		state = deliveryDone
	}

	machine := newJobMachine()
	machine.pickup(time.Now())
	machine.complete(time.Now())
	result := JobResult{ID: c.job.ID, Result: cp.PartialResult, History: machine.history()}
	if err := journal.Record(ctx, result); err != nil {
		return err
	}
	emit(result)
	return stream.ack(context.WithoutCancel(ctx), c.entry)
}

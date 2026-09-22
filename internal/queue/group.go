package queue

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"coldharbour/internal/checkpoint"
	"coldharbour/internal/events"
	"coldharbour/internal/journal"
	"coldharbour/internal/runner"
	"coldharbour/internal/seal"

	"github.com/redis/go-redis/v9"
)

const workerGroup = "worker-group"

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

func RunGroup(ctx context.Context, stream *jobStream, cfg WorkerConfig, reg *runner.Registry, store journal.Journal, keys seal.KeyStore, receipts seal.ReceiptStore, priv ed25519.PrivateKey, emit func(journal.JobResult)) error {
	if store == nil {
		panic("nil journal")
	}
	if keys == nil {
		panic("nil key store")
	}
	if receipts == nil {
		panic("nil receipt store")
	}
	if len(priv) != ed25519.PrivateKeySize {
		panic("nil signing key")
	}
	if reg == nil {
		panic("nil registry")
	}
	if err := cfg.valid(); err != nil {
		return err
	}
	hash := &memHash{rdb: stream.rdb, keys: keys}
	purge := &purger{rdb: stream.rdb, keys: keys, receipts: receipts, priv: priv}
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
			if err := dispatch(ctx, stream, hash, purge, reg, msg, store, emit); err != nil {
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
				if err := dispatch(ctx, stream, hash, purge, reg, msg, store, emit); err != nil {
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

func dispatch(ctx context.Context, stream *jobStream, hash *memHash, purge *purger, reg *runner.Registry, msg redis.XMessage, store journal.Journal, emit func(journal.JobResult)) error {
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
	return handle(ctx, stream, hash, purge, reg, c, store, emit)
}

func jobInput(raw string) map[string]any {
	var input map[string]any
	if err := json.Unmarshal([]byte(raw), &input); err == nil && input != nil {
		return input
	}
	return map[string]any{"input": raw}
}

func handle(ctx context.Context, stream *jobStream, hash *memHash, purge *purger, reg *runner.Registry, c claimed, store journal.Journal, emit func(journal.JobResult)) error {
	st, err := hash.load(ctx, c.job.ID)
	if err != nil {
		return err
	}
	if st == nil {
		_, loadErr := store.Load(ctx, c.job.ID)
		if loadErr == nil {
			return stream.ack(context.WithoutCancel(ctx), c.entry)
		}
		if !errors.Is(loadErr, journal.ErrUnknownStoredJob) {
			return loadErr
		}
		if _, err := hash.keys.Ensure(ctx, journal.DurableIDFor(c.job.ID)); err != nil {
			return err
		}
	}

	jobType := c.fields["job_type"]
	if jobType == "" {
		jobType = "redact"
	}
	jr, ok := reg.Get(jobType)
	if !ok {
		return fmt.Errorf("unknown job_type %q", jobType)
	}

	cp := runner.NewCheckpointRecorder(
		func() (int, map[string]any, bool) {
			if st == nil {
				return 0, nil, false
			}
			return st.Step, st.Partial, true
		},
		func(step int, partial map[string]any) error {
			next := memState{JobID: c.job.ID, Step: step, Partial: partial}
			var err error
			if step == 1 {
				err = hash.saveStep1(ctx, next)
			} else {
				err = hash.save(ctx, next)
			}
			if err != nil {
				return err
			}
			st = &next
			if step == 1 {
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
			}
			return nil
		},
	)

	out, err := jr.Run(ctx, jobInput(c.job.Input), cp)
	if err != nil {
		return err
	}

	if c.fail {
		result := journal.Fail(c.job.ID, out)
		result.TenantID = c.job.TenantID
		result.JobType = jr.JobType()
		return (&deadLetters{rdb: stream.rdb}).fail(context.WithoutCancel(ctx), c, result, store, purge)
	}
	result := journal.Succeed(c.job.ID, out)
	result.TenantID = c.job.TenantID
	result.JobType = jr.JobType()
	purge.signOutput(&result)
	if err := store.Record(ctx, result); err != nil {
		return err
	}
	if err := purge.run(ctx, c.job.ID); err != nil {
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

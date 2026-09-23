package queue

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"coldharbour/internal/events"
	"coldharbour/internal/journal"
	"coldharbour/internal/runner"
	"coldharbour/internal/seal"

	"github.com/redis/go-redis/v9"
)

const workerGroup = "worker-group"

type infraError struct {
	err error
}

func (e infraError) Error() string {
	if e.err == nil {
		return "infrastructure"
	}
	return e.err.Error()
}

func (e infraError) Unwrap() error {
	return e.err
}

type claimed struct {
	entry  StreamID
	job    Job
	crash  CrashPoint
	fail   bool
	fields map[string]string
}

func parseClaim(entry StreamID, fields map[string]string) (claimed, error) {
	job, err := parseJob(entry, fields)
	if err != nil {
		return claimed{}, err
	}
	return claimed{
		entry:  entry,
		job:    job,
		crash:  CrashNever,
		fields: fields,
	}, nil
}

var ErrSimulatedCrash = errors.New("simulated crash after step 1 checkpoint")

type CrashPoint int

const (
	CrashNever           CrashPoint = 0
	CrashAfterStep1      CrashPoint = 1
	CrashAfterRecord     CrashPoint = 2
	CrashAfterKeyDestroy CrashPoint = 3
)

type Hooks struct {
	CrashAfterStep1      func(jobID string) bool
	CrashAfterRecord     func(jobID string) bool
	CrashAfterKeyDestroy func(jobID string) bool
	Fail                 func(jobID string) bool
}

func applyHooks(c *claimed, hooks Hooks) {
	switch {
	case hooks.CrashAfterStep1 != nil && hooks.CrashAfterStep1(c.job.ID):
		c.crash = CrashAfterStep1
	case hooks.CrashAfterRecord != nil && hooks.CrashAfterRecord(c.job.ID):
		c.crash = CrashAfterRecord
	case hooks.CrashAfterKeyDestroy != nil && hooks.CrashAfterKeyDestroy(c.job.ID):
		c.crash = CrashAfterKeyDestroy
	}
	if hooks.Fail != nil && hooks.Fail(c.job.ID) {
		c.fail = true
	}
}

func commitJob(ctx context.Context, c claimed, result journal.JobResult, store journal.Journal, purge *purger) error {
	if err := store.Record(ctx, result); err != nil {
		return err
	}
	if c.crash == CrashAfterRecord {
		return ErrSimulatedCrash
	}
	return purge.run(ctx, c.job.ID, c.crash, c.job.TenantID)
}

func (s *jobStream) ensureGroup(ctx context.Context) error {
	err := s.rdb.XGroupCreateMkStream(ctx, jobsStreamKey, workerGroup, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (s *jobStream) enqueue(ctx context.Context, job Job) error {
	values := map[string]any{"input": job.Input}
	if job.ID != "" {
		values["id"] = job.ID
	}
	if job.TenantID != "" {
		values["tenant_id"] = job.TenantID
	}
	if job.JobType != "" {
		values["job_type"] = job.JobType
	}
	return s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: values,
	}).Err()
}

func (s *jobStream) finish(ctx context.Context, id StreamID) error {
	_, err := s.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.XAck(ctx, jobsStreamKey, workerGroup, id.String())
		pipe.XDel(ctx, jobsStreamKey, id.String())
		return nil
	})
	return err
}

func PrepareGroup(ctx context.Context, stream *jobStream) error {
	return stream.ensureGroup(ctx)
}

type Deps struct {
	Registry   *runner.Registry
	Journal    journal.Journal
	Keys       seal.KeyStore
	Receipts   seal.ReceiptStore
	SigningKey ed25519.PrivateKey
	Emit       func(journal.JobResult)
	Hooks      Hooks
}

func (d Deps) ready() error {
	switch {
	case d.Journal == nil:
		return errors.New("queue: Journal is required")
	case d.Keys == nil:
		return errors.New("queue: Keys is required")
	case d.Receipts == nil:
		return errors.New("queue: Receipts is required")
	case len(d.SigningKey) != ed25519.PrivateKeySize:
		return errors.New("queue: SigningKey is required")
	case d.Registry == nil:
		return errors.New("queue: Registry is required")
	default:
		return nil
	}
}

func RunGroup(ctx context.Context, stream *jobStream, cfg WorkerConfig, deps Deps) error {
	if err := deps.ready(); err != nil {
		return err
	}
	if err := cfg.valid(); err != nil {
		return err
	}
	reg := deps.Registry
	store := deps.Journal
	keys := deps.Keys
	receipts := deps.Receipts
	priv := deps.SigningKey
	emit := deps.Emit
	if emit == nil {
		emit = func(journal.JobResult) {}
	}
	hooks := deps.Hooks
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
			if err := dispatch(ctx, stream, hash, purge, reg, msg, store, emit, hooks); err != nil {
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
				if err := dispatch(ctx, stream, hash, purge, reg, msg, store, emit, hooks); err != nil {
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

func dispatch(ctx context.Context, stream *jobStream, hash *memHash, purge *purger, reg *runner.Registry, msg redis.XMessage, store journal.Journal, emit func(journal.JobResult), hooks Hooks) error {
	id, err := ParseStreamID(msg.ID)
	if err != nil {
		return err
	}
	fields := valuesToFields(msg.Values)
	c, err := parseClaim(id, fields)
	if errors.Is(err, errMissingInput) {
		return stream.finish(context.WithoutCancel(ctx), id)
	}
	if errors.Is(err, errMissingTenant) {
		return deadLetterMissingTenant(ctx, stream, purge, id, fields)
	}
	if err != nil {
		return err
	}
	return handle(ctx, stream, hash, purge, reg, c, store, emit, hooks)
}

func deadLetterMissingTenant(ctx context.Context, stream *jobStream, purge *purger, id StreamID, fields map[string]string) error {
	jobID := fields["id"]
	if jobID == "" {
		jobID = id.String()
	}
	if err := purge.keys.Destroy(ctx, journal.DurableIDFor(jobID), time.Now().UTC()); err != nil {
		return err
	}
	c := claimed{entry: id, job: Job{ID: jobID}, fields: fields}
	_, _, err := (&deadLetters{rdb: stream.rdb}).settle(ctx, c, true)
	return err
}

func openJobInput(ctx context.Context, keys seal.KeyStore, jobID, raw string) (string, error) {
	key, ok, err := keys.Load(ctx, journal.DurableIDFor(jobID))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errInputKeyMissing
	}
	plain, err := seal.Open(key, raw)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func jobInput(raw string) map[string]any {
	var input map[string]any
	if err := json.Unmarshal([]byte(raw), &input); err == nil && input != nil {
		return input
	}
	return map[string]any{"input": raw}
}

func finishJournaled(ctx context.Context, stream *jobStream, purge *purger, c claimed, store journal.Journal) (bool, error) {
	_, err := store.Load(ctx, c.job.ID)
	if errors.Is(err, journal.ErrUnknownStoredJob) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := purge.run(ctx, c.job.ID, CrashNever, c.job.TenantID); err != nil {
		return false, err
	}
	if err := stream.finish(context.WithoutCancel(ctx), c.entry); err != nil {
		return false, err
	}
	return true, nil
}

func handle(ctx context.Context, stream *jobStream, hash *memHash, purge *purger, reg *runner.Registry, c claimed, store journal.Journal, emit func(journal.JobResult), hooks Hooks) error {
	started := time.Now()
	defer observeJob(started)
	applyHooks(&c, hooks)
	finished, err := finishJournaled(ctx, stream, purge, c, store)
	if err != nil || finished {
		return err
	}
	st, err := hash.load(ctx, c.job.ID)
	if err != nil {
		return err
	}

	jobType := c.fields["job_type"]
	if jobType == "" {
		jobType = "redact"
	}
	jr, ok := reg.Get(jobType)
	if !ok {
		return (&deadLetters{rdb: stream.rdb}).bury(ctx, c, jobType, "unknown job_type", store, purge)
	}
	plain, err := openJobInput(ctx, hash.keys, c.job.ID, c.job.Input)
	if err != nil {
		if errors.Is(err, seal.ErrBadSealed) {
			return (&deadLetters{rdb: stream.rdb}).bury(ctx, c, jr.JobType(), "input is not sealed", store, purge)
		}
		if errors.Is(err, errInputKeyMissing) {
			return (&deadLetters{rdb: stream.rdb}).bury(ctx, c, jr.JobType(), "input key missing", store, purge)
		}
		return err
	}
	c.job.Input = plain

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
				return infraError{err: err}
			}
			st = &next
			if step == 1 {
				if err := events.Publish(ctx, stream.rdb, events.JobEvent{
					TenantID: c.job.TenantID,
					JobID:    c.job.ID,
					Status:   string(journal.RUNNING),
				}); err != nil {
					return infraError{err: err}
				}
				if c.crash == CrashAfterStep1 {
					return ErrSimulatedCrash
				}
			}
			return nil
		},
	)

	out, err := jr.Run(ctx, jobInput(c.job.Input), cp)
	if err != nil {
		if errors.Is(err, ErrSimulatedCrash) {
			return err
		}
		var infra infraError
		if errors.As(err, &infra) {
			return err
		}
		return (&deadLetters{rdb: stream.rdb}).bury(ctx, c, jr.JobType(), "runner failed", store, purge)
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
	if err := purge.signOutput(&result); err != nil {
		return err
	}
	if err := commitJob(ctx, c, result, store, purge); err != nil {
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
	jobsCompleted.Inc()
	return stream.finish(context.WithoutCancel(ctx), c.entry)
}

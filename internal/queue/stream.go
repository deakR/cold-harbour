package queue

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"coldharbour/internal/journal"
	"coldharbour/internal/seal"

	"github.com/redis/go-redis/v9"
)

const jobsStreamKey = "coldharbour:jobs"

var (
	errInvalidStreamID = errors.New("stream id must be millis-seq")
	errMissingInput    = errors.New("missing input field")
	errMissingTenant   = errors.New("missing tenant_id")
	errInputKeyMissing = errors.New("input key missing")
)

type StreamID struct {
	millis, seq uint64
}

func StreamIDZero() StreamID {
	return StreamID{}
}

func ParseStreamID(s string) (StreamID, error) {
	millis, seq, ok := strings.Cut(s, "-")
	if !ok || millis == "" || seq == "" || strings.Contains(seq, "-") {
		return StreamID{}, errInvalidStreamID
	}
	m, err := strconv.ParseUint(millis, 10, 64)
	if err != nil {
		return StreamID{}, errInvalidStreamID
	}
	n, err := strconv.ParseUint(seq, 10, 64)
	if err != nil {
		return StreamID{}, errInvalidStreamID
	}
	return StreamID{millis: m, seq: n}, nil
}

func (id StreamID) String() string {
	return fmt.Sprintf("%d-%d", id.millis, id.seq)
}

func parseJob(entryID StreamID, fields map[string]string) (Job, error) {
	input, ok := fields["input"]
	if !ok || input == "" {
		return Job{}, errMissingInput
	}
	id := fields["id"]
	if id == "" {
		id = entryID.String()
	}
	tenant := fields["tenant_id"]
	if tenant == "" {
		return Job{}, errMissingTenant
	}
	return Job{ID: id, Input: input, TenantID: tenant}, nil
}

func RedisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:6379"
}

type jobStream struct {
	rdb *redis.Client
}

func redisOptions(addr string) *redis.Options {
	opt := &redis.Options{Addr: addr}
	if pw := os.Getenv("REDIS_PASSWORD"); pw != "" {
		opt.Password = pw
	}
	if os.Getenv("REDIS_TLS") == "1" {
		cfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if caPath := os.Getenv("REDIS_CA"); caPath != "" {
			pem, err := os.ReadFile(caPath) //#nosec G304 G703 -- REDIS_CA is an operator path
			if err != nil {
				panic(err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				panic("REDIS_CA is not a PEM certificate")
			}
			cfg.RootCAs = pool
		}
		opt.TLSConfig = cfg
	}
	return opt
}

func OpenJobs(addr string) *jobStream {
	return &jobStream{rdb: redis.NewClient(redisOptions(addr))}
}

func NewRedisClient(addr string) *redis.Client {
	return redis.NewClient(redisOptions(addr))
}

func JobRunning(ctx context.Context, rdb redis.Cmdable, jobID string) (bool, error) {
	step, err := rdb.HGet(ctx, memKey(jobID), memFieldStep).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return step != "", nil
}

func (s *jobStream) Add(ctx context.Context, keys seal.KeyStore, job Job) error {
	if keys == nil {
		return errors.New("key store is required")
	}
	if job.ID == "" {
		return errors.New("job id is required")
	}
	key, err := keys.Ensure(ctx, journal.DurableIDFor(job.ID))
	if err != nil {
		return err
	}
	sealed, err := seal.Seal(key, []byte(job.Input))
	if err != nil {
		return err
	}
	job.Input = sealed
	return s.enqueue(ctx, job)
}

func (s *jobStream) Len(ctx context.Context) (int64, error) {
	return s.rdb.XLen(ctx, jobsStreamKey).Result()
}

func valuesToFields(values map[string]any) map[string]string {
	fields := make(map[string]string, len(values))
	for k, v := range values {
		switch t := v.(type) {
		case string:
			fields[k] = t
		case []byte:
			fields[k] = string(t)
		default:
			fields[k] = fmt.Sprint(t)
		}
	}
	return fields
}

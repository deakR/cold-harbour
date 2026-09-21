package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

const jobsStreamKey = "coldharbour:jobs"

var (
	errInvalidStreamID = errors.New("stream id must be millis-seq")
	errMissingInput    = errors.New("missing input field")
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
	return Job{ID: id, Input: input}, nil
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:6379"
}

type jobStream struct {
	rdb *redis.Client
}

func openJobs(addr string) *jobStream {
	return &jobStream{rdb: redis.NewClient(&redis.Options{Addr: addr})}
}

func (s *jobStream) Add(ctx context.Context, job Job) error {
	values := map[string]any{"input": job.Input}
	if job.ID != "" {
		values["id"] = job.ID
	}
	return s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: values,
	}).Err()
}

func (s *jobStream) Len(ctx context.Context) (int64, error) {
	return s.rdb.XLen(ctx, jobsStreamKey).Result()
}

func seedIfEmpty(ctx context.Context, stream *jobStream, jobs []Job) error {
	n, err := stream.Len(ctx)
	if err != nil {
		return err
	}
	if n != 0 {
		return nil
	}
	for _, job := range jobs {
		if err := stream.Add(ctx, job); err != nil {
			return err
		}
	}
	return nil
}

func runStream(ctx context.Context, stream *jobStream, emit func(JobResult)) error {
	cursor := StreamIDZero()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		streams, err := stream.rdb.XRead(ctx, &redis.XReadArgs{
			Streams: []string{jobsStreamKey, cursor.String()},
			Block:   0,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			return err
		}
		for _, xs := range streams {
			for _, msg := range xs.Messages {
				id, err := ParseStreamID(msg.ID)
				if err != nil {
					return err
				}
				cursor = id
				job, err := parseJob(id, valuesToFields(msg.Values))
				if err != nil {
					continue
				}
				emit(processJob(job))
			}
		}
	}
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

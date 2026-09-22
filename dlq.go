package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const dlqStreamKey = "coldharbour:jobs:dlq"

var (
	errEmptyDLQ  = errors.New("dlq is empty")
	errRetryLive = errors.New("retry key is live")
	errSettle    = errors.New("settle returned an unexpected value")
)

const settleScript = `
local key = KEYS[1]
local main = KEYS[2]
local dlq = KEYS[3]
local group = ARGV[1]
local entry = ARGV[2]
local raw = redis.call('GET', key)
local n = tonumber(raw)
if not (n and n >= 3) then
  n = redis.call('INCR', key)
end
if n < 3 then
  redis.call('XADD', main, '*', unpack(ARGV, 3))
  redis.call('XACK', main, group, entry)
  return {'retry', n}
end
redis.call('XADD', dlq, '*', unpack(ARGV, 3))
redis.call('XACK', main, group, entry)
redis.call('DEL', key)
return {'bury', n}
`

type deadLetters struct {
	rdb *redis.Client
}

func retryKey(jobID string) string {
	return "retry:" + jobID
}

func (d *deadLetters) fail(ctx context.Context, c claimed, result JobResult, journal Journal) error {
	n, ok, err := d.peek(ctx, c.job.ID)
	if err != nil {
		return err
	}
	if ok && n >= 2 {
		if err := journal.Record(ctx, result); err != nil {
			return err
		}
	}
	kind, attempt, err := d.settle(ctx, c)
	if err != nil {
		return err
	}
	if kind == "retry" {
		fmt.Printf("attempt %d %s\n", attempt, c.job.ID)
	}
	return nil
}

func (d *deadLetters) peek(ctx context.Context, jobID string) (int, bool, error) {
	raw, err := d.rdb.Get(ctx, retryKey(jobID)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false, err
	}
	return n, true, nil
}

func (d *deadLetters) settle(ctx context.Context, c claimed) (string, int, error) {
	args := make([]any, 0, 2+2*len(c.fields))
	args = append(args, workerGroup, c.entry.String())
	for k, v := range c.fields {
		args = append(args, k, v)
	}
	raw, err := d.rdb.Eval(ctx, settleScript, []string{
		retryKey(c.job.ID),
		jobsStreamKey,
		dlqStreamKey,
	}, args...).Result()
	if err != nil {
		return "", 0, err
	}
	return parseSettle(raw)
}

func parseSettle(raw any) (string, int, error) {
	row, ok := raw.([]any)
	if !ok || len(row) != 2 {
		return "", 0, errSettle
	}
	kind, ok := row[0].(string)
	if !ok || (kind != "retry" && kind != "bury") {
		return "", 0, errSettle
	}
	n, ok := luaInt(row[1])
	if !ok {
		return "", 0, errSettle
	}
	return kind, n, nil
}

func luaInt(v any) (int, bool) {
	switch n := v.(type) {
	case int64:
		return int(n), true
	case int:
		return n, true
	case string:
		parsed, err := strconv.Atoi(n)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func (d *deadLetters) redrive(ctx context.Context) error {
	entries, err := d.rdb.XRangeN(ctx, dlqStreamKey, "-", "+", 1).Result()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return errEmptyDLQ
	}
	fields := valuesToFields(entries[0].Values)
	entryID, err := ParseStreamID(entries[0].ID)
	if err != nil {
		return err
	}
	job, err := parseJob(entryID, fields)
	if err != nil {
		return err
	}
	jobID := job.ID
	set, err := d.rdb.SetNX(ctx, retryKey(jobID), 0, 0).Result()
	if err != nil {
		return err
	}
	if !set {
		raw, err := d.rdb.Get(ctx, retryKey(jobID)).Result()
		if err != nil {
			return err
		}
		if raw != "0" {
			return errRetryLive
		}
	}
	values := make(map[string]any, len(fields))
	for k, v := range fields {
		values[k] = v
	}
	return d.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: values,
	}).Err()
}

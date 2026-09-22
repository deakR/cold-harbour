package queue

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"coldharbour/internal/events"
	"coldharbour/internal/journal"

	"github.com/redis/go-redis/v9"
)

const dlqStreamKey = "coldharbour:jobs:dlq"

var errSettle = errors.New("settle returned an unexpected value")

const settleScript = `
local key = KEYS[1]
local main = KEYS[2]
local dlq = KEYS[3]
local group = ARGV[1]
local entry = ARGV[2]
local force = ARGV[3]
local n
if force == '1' then
  n = 3
else
  local raw = redis.call('GET', key)
  n = tonumber(raw)
  if not (n and n >= 3) then
    n = redis.call('INCR', key)
  end
end
if force ~= '1' and n < 3 then
  redis.call('XADD', main, '*', unpack(ARGV, 4))
  redis.call('XACK', main, group, entry)
  redis.call('XDEL', main, entry)
  return {'retry', n}
end
local fields = {}
local i = 4
while i + 1 <= #ARGV do
  if ARGV[i] ~= 'input' then
    fields[#fields + 1] = ARGV[i]
    fields[#fields + 1] = ARGV[i + 1]
  end
  i = i + 2
end
if #fields > 0 then
  redis.call('XADD', dlq, '*', unpack(fields))
end
redis.call('XACK', main, group, entry)
redis.call('XDEL', main, entry)
redis.call('DEL', key)
return {'bury', n}
`

type deadLetters struct {
	rdb *redis.Client
}

func retryKey(jobID string) string {
	return "retry:" + jobID
}

func (d *deadLetters) fail(ctx context.Context, c claimed, result journal.JobResult, store journal.Journal, purge *purger) error {
	n, ok, err := d.peek(ctx, c.job.ID)
	if err != nil {
		return err
	}
	if ok && n >= 2 {
		purge.signOutput(&result)
		if err := store.Record(ctx, result); err != nil {
			return err
		}
		if err := purge.run(ctx, c.job.ID); err != nil {
			return err
		}
		if err := events.Publish(ctx, d.rdb, events.JobEvent{
			TenantID: result.TenantID,
			JobID:    result.ID,
			Status:   string(journal.FAILED),
		}); err != nil {
			return err
		}
	}
	kind, attempt, err := d.settle(ctx, c, false)
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

func (d *deadLetters) bury(ctx context.Context, c claimed, jobType, reason string, store journal.Journal, purge *purger) error {
	if jobType == "" {
		jobType = "redact"
	}
	result := journal.Fail(c.job.ID, map[string]any{"error": reason})
	result.TenantID = c.job.TenantID
	result.JobType = jobType
	purge.signOutput(&result)
	if err := store.Record(ctx, result); err != nil {
		return err
	}
	if err := purge.run(ctx, c.job.ID); err != nil {
		return err
	}
	if err := events.Publish(ctx, d.rdb, events.JobEvent{
		TenantID: result.TenantID,
		JobID:    result.ID,
		Status:   string(journal.FAILED),
	}); err != nil {
		return err
	}
	_, _, err := d.settle(ctx, c, true)
	return err
}

func (d *deadLetters) settle(ctx context.Context, c claimed, force bool) (string, int, error) {
	flag := "0"
	if force {
		flag = "1"
	}
	args := make([]any, 0, 3+2*len(c.fields))
	args = append(args, workerGroup, c.entry.String(), flag)
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
		return int(n), true
	case string:
		parsed, err := strconv.Atoi(n)
		return parsed, err == nil
	default:
		return 0, false
	}
}


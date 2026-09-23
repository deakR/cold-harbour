package queue

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
)

const scrubDLQScript = `
local old = ARGV[1]
if #ARGV == 1 then
  return redis.call('XDEL', KEYS[1], old)
end
redis.call('XADD', KEYS[1], '*', unpack(ARGV, 2))
return redis.call('XDEL', KEYS[1], old)
`

type ScrubKept struct {
	ID     string
	Reason string
}

type ScrubReport struct {
	MainDeleted  []string
	MainKept     []ScrubKept
	DLQRewritten []string
	DLQRemoved   []string
}

// ScrubPlaintext removes input left by workers that acked a job without
// deleting it. A main-stream entry is deleted only when worker-group will
// not deliver it again: its id is at or behind the group's last-delivered
// id, and it is not pending. Pending and unread jobs keep their input.
// Dead-letter entries stay, with the input field removed.
func (s *jobStream) ScrubPlaintext(ctx context.Context) (ScrubReport, error) {
	var report ScrubReport
	if err := s.scrubMain(ctx, &report); err != nil {
		return report, err
	}
	if err := s.scrubDLQ(ctx, &report); err != nil {
		return report, err
	}
	return report, nil
}

func (s *jobStream) scrubMain(ctx context.Context, report *ScrubReport) error {
	entries, err := s.rdb.XRange(ctx, jobsStreamKey, "-", "+").Result()
	if err != nil {
		if missingRedisKey(err) {
			return nil
		}
		return err
	}
	tip, hasGroup, err := s.groupTip(ctx)
	if err != nil {
		return err
	}
	pending := map[string]struct{}{}
	if hasGroup {
		pending, err = s.pendingIDs(ctx)
		if err != nil {
			return err
		}
	}
	var deleteIDs []string
	for _, entry := range entries {
		fields := valuesToFields(entry.Values)
		if _, ok := fields["input"]; !ok {
			continue
		}
		if !hasGroup {
			report.MainKept = append(report.MainKept, ScrubKept{ID: entry.ID, Reason: "no worker-group"})
			continue
		}
		if _, ok := pending[entry.ID]; ok {
			report.MainKept = append(report.MainKept, ScrubKept{ID: entry.ID, Reason: "pending"})
			continue
		}
		settled, err := streamIDAtMost(entry.ID, tip)
		if err != nil {
			return err
		}
		if !settled {
			report.MainKept = append(report.MainKept, ScrubKept{ID: entry.ID, Reason: "not yet delivered"})
			continue
		}
		deleteIDs = append(deleteIDs, entry.ID)
	}
	if len(deleteIDs) == 0 {
		return nil
	}
	if err := s.rdb.XDel(ctx, jobsStreamKey, deleteIDs...).Err(); err != nil {
		return err
	}
	report.MainDeleted = deleteIDs
	return nil
}

func (s *jobStream) scrubDLQ(ctx context.Context, report *ScrubReport) error {
	entries, err := s.rdb.XRange(ctx, dlqStreamKey, "-", "+").Result()
	if err != nil {
		if missingRedisKey(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		fields := valuesToFields(entry.Values)
		if _, ok := fields["input"]; !ok {
			continue
		}
		args := make([]any, 0, 1+2*(len(fields)-1))
		args = append(args, entry.ID)
		for k, v := range fields {
			if k == "input" {
				continue
			}
			args = append(args, k, v)
		}
		if err := s.rdb.Eval(ctx, scrubDLQScript, []string{dlqStreamKey}, args...).Err(); err != nil {
			return err
		}
		if len(args) == 1 {
			report.DLQRemoved = append(report.DLQRemoved, entry.ID)
			continue
		}
		report.DLQRewritten = append(report.DLQRewritten, entry.ID)
	}
	return nil
}

func (s *jobStream) groupTip(ctx context.Context) (string, bool, error) {
	groups, err := s.rdb.XInfoGroups(ctx, jobsStreamKey).Result()
	if err != nil {
		if missingRedisKey(err) {
			return "", false, nil
		}
		return "", false, err
	}
	for _, group := range groups {
		if group.Name == workerGroup {
			return group.LastDeliveredID, true, nil
		}
	}
	return "", false, nil
}

func (s *jobStream) pendingIDs(ctx context.Context) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	start := "-"
	for {
		rows, err := s.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: jobsStreamKey,
			Group:  workerGroup,
			Start:  start,
			End:    "+",
			Count:  128,
		}).Result()
		if err != nil {
			if missingRedisKey(err) || strings.Contains(err.Error(), "NOGROUP") {
				return out, nil
			}
			return nil, err
		}
		if len(rows) == 0 {
			return out, nil
		}
		for _, row := range rows {
			out[row.ID] = struct{}{}
		}
		if len(rows) < 128 {
			return out, nil
		}
		start = "(" + rows[len(rows)-1].ID
	}
}

func streamIDAtMost(id, tip string) (bool, error) {
	left, err := ParseStreamID(id)
	if err != nil {
		return false, err
	}
	right, err := ParseStreamID(tip)
	if err != nil {
		return false, err
	}
	if left.millis != right.millis {
		return left.millis < right.millis, nil
	}
	return left.seq <= right.seq, nil
}

func missingRedisKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such key")
}

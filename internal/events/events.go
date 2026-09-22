package events

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

const Channel = "coldharbour:events"

type JobEvent struct {
	TenantID string `json:"tenantId"`
	JobID    string `json:"jobId"`
	Status   string `json:"status"`
}

func Publish(ctx context.Context, rdb redis.Cmdable, e JobEvent) error {
	if e.TenantID == "" {
		return nil
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return rdb.Publish(ctx, Channel, payload).Err()
}

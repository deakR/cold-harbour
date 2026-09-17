package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"coldharbor/worker-engine/internal/model"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// appendAuditEnvelope writes terminal execution data to a durable Redis Stream.
// The control plane acknowledges and deletes entries only after PostgreSQL
// persistence succeeds.
func appendAuditEnvelope(
	ctx context.Context,
	client redis.UniversalClient,
	streamName string,
	deadDrop *model.DeadDropPayload,
	finalState string,
	details string,
) error {
	if streamName == "" {
		streamName = "coldharbor:audits"
	}
	payload, err := json.Marshal(deadDrop)
	if err != nil {
		return fmt.Errorf("marshal audit payload: %w", err)
	}
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName,
		Values: map[string]any{
			"auditId":    uuid.NewString(),
			"finalState": finalState,
			"details":    details,
			"data":       string(payload),
		},
	}).Err(); err != nil {
		return fmt.Errorf("append audit to stream %s: %w", streamName, err)
	}
	return nil
}

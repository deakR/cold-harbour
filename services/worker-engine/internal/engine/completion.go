package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const completionNamespace = "coldharbor:worker-engine"

var finalizeCompletionScript = redis.NewScript(`
if redis.call("GET", KEYS[2]) ~= ARGV[1] then
	return 0
end
redis.call("DEL", KEYS[1])
redis.call("SET", KEYS[3], "finalized", "PX", ARGV[2])
redis.call("DEL", KEYS[2])
return 1
`)

var acquireLeaseScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
	return -1
end
if redis.call("SET", KEYS[2], ARGV[1], "NX", "PX", ARGV[2]) then
	return 1
end
return 0
`)

var releaseLeaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// CompletionGuard prevents concurrent/replayed delivery from executing a job
// after its archive has been sealed and its scratchpad has been purged.
type CompletionGuard struct {
	client redis.UniversalClient
	ttl    time.Duration
}

func NewCompletionGuard(client redis.UniversalClient, ttl time.Duration) *CompletionGuard {
	if ttl < 30*24*time.Hour {
		ttl = 30 * 24 * time.Hour
	}
	return &CompletionGuard{client: client, ttl: ttl}
}

func completionSuffix(stream, group, messageID string) string {
	sum := sha256.Sum256([]byte(stream + "\x00" + group + "\x00" + messageID))
	return hex.EncodeToString(sum[:])
}

func (g *CompletionGuard) markerKey(stream, group, messageID string) string {
	return fmt.Sprintf("%s:completed:%s", completionNamespace, completionSuffix(stream, group, messageID))
}

func (g *CompletionGuard) leaseKey(stream, group, messageID string) string {
	return fmt.Sprintf("%s:lease:%s", completionNamespace, completionSuffix(stream, group, messageID))
}

func (g *CompletionGuard) auditKey(stream, group, messageID string) string {
	return fmt.Sprintf("%s:audit-published:%s", completionNamespace, completionSuffix(stream, group, messageID))
}

func (g *CompletionGuard) IsCompleted(ctx context.Context, stream, group, messageID string) (bool, error) {
	n, err := g.client.Exists(ctx, g.markerKey(stream, group, messageID)).Result()
	if err != nil {
		return false, fmt.Errorf("check completion marker: %w", err)
	}
	return n > 0, nil
}

func (g *CompletionGuard) NeedsTerminalEvent(ctx context.Context, stream, group, messageID string) (bool, error) {
	value, err := g.client.Get(ctx, g.markerKey(stream, group, messageID)).Result()
	if err != nil {
		return false, fmt.Errorf("read completion marker: %w", err)
	}
	return value != "published", nil
}

func (g *CompletionGuard) MarkTerminalEventPublished(
	ctx context.Context, stream, group, messageID string,
) error {
	if err := g.client.Set(ctx, g.markerKey(stream, group, messageID), "published", g.ttl).Err(); err != nil {
		return fmt.Errorf("mark terminal event published: %w", err)
	}
	return nil
}

func (g *CompletionGuard) IsAuditPublished(ctx context.Context, stream, group, messageID string) (bool, error) {
	n, err := g.client.Exists(ctx, g.auditKey(stream, group, messageID)).Result()
	if err != nil {
		return false, fmt.Errorf("check audit marker: %w", err)
	}
	return n > 0, nil
}

func (g *CompletionGuard) MarkAuditPublished(ctx context.Context, stream, group, messageID string) error {
	if err := g.client.Set(ctx, g.auditKey(stream, group, messageID), "1", g.ttl).Err(); err != nil {
		return fmt.Errorf("mark audit published: %w", err)
	}
	return nil
}

func (g *CompletionGuard) Acquire(
	ctx context.Context,
	stream, group, messageID string,
	leaseTTL time.Duration,
) (token string, acquired bool, completed bool, err error) {
	if leaseTTL < 5*time.Minute {
		leaseTTL = 5 * time.Minute
	}
	token = uuid.NewString()
	result, err := acquireLeaseScript.Run(ctx, g.client,
		[]string{g.markerKey(stream, group, messageID), g.leaseKey(stream, group, messageID)},
		token, leaseTTL.Milliseconds(),
	).Int()
	if err != nil {
		return "", false, false, fmt.Errorf("acquire execution lease: %w", err)
	}
	return token, result == 1, result == -1, nil
}

// Finalize atomically purges scratch state and writes the durable completion
// marker, while fencing stale processors with the lease token.
func (g *CompletionGuard) Finalize(
	ctx context.Context,
	stream, group, messageID, scratchpadKey, token string,
) error {
	result, err := finalizeCompletionScript.Run(ctx, g.client,
		[]string{scratchpadKey, g.leaseKey(stream, group, messageID), g.markerKey(stream, group, messageID)},
		token, g.ttl.Milliseconds(),
	).Int()
	if err != nil {
		return fmt.Errorf("finalize completion: %w", err)
	}
	if result != 1 {
		return fmt.Errorf("finalize completion: execution lease lost")
	}
	return nil
}

func (g *CompletionGuard) Release(ctx context.Context, stream, group, messageID, token string) error {
	if token == "" {
		return nil
	}
	if err := releaseLeaseScript.Run(ctx, g.client,
		[]string{g.leaseKey(stream, group, messageID)}, token,
	).Err(); err != nil && err != redis.Nil {
		return fmt.Errorf("release execution lease: %w", err)
	}
	return nil
}

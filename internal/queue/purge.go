package queue

import (
	"context"
	"crypto/ed25519"
	"errors"
	"time"

	"coldharbour/internal/journal"
	"coldharbour/internal/seal"

	"github.com/redis/go-redis/v9"
)

var errMissingReceipt = errors.New("purge receipt missing after conflict")

type purger struct {
	rdb      *redis.Client
	keys     seal.KeyStore
	receipts seal.ReceiptStore
	priv     ed25519.PrivateKey
}

func (p *purger) run(ctx context.Context, redisJobID string, crash CrashPoint, tenantID string) error {
	durable := journal.DurableIDFor(redisJobID)
	if existing, ok, err := p.receipts.Get(ctx, durable); err != nil {
		return err
	} else if ok {
		if err := p.keys.Destroy(ctx, durable, existing.PurgedAt); err != nil {
			return err
		}
		return p.dropCheckpoint(ctx, redisJobID, crash)
	}

	purgedAt := time.Now().UTC().Truncate(time.Microsecond)
	msg, err := seal.PurgeMessage(redisJobID, purgedAt)
	if err != nil {
		return err
	}
	pub := p.priv.Public().(ed25519.PublicKey)
	sig := seal.Sign(p.priv, msg)
	inserted, err := p.receipts.Insert(ctx, seal.Receipt{
		JobID:        durable,
		RedisJobID:   redisJobID,
		PurgedAt:     purgedAt,
		Signature:    sig,
		SigningKeyID: seal.SigningKeyID(pub),
		TenantID:     tenantID,
	})
	if err != nil {
		return err
	}
	if !inserted {
		existing, ok, err := p.receipts.Get(ctx, durable)
		if err != nil {
			return err
		}
		if !ok {
			return errMissingReceipt
		}
		purgedAt = existing.PurgedAt
	}
	if err := p.keys.Destroy(ctx, durable, purgedAt); err != nil {
		return err
	}
	return p.dropCheckpoint(ctx, redisJobID, crash)
}

func (p *purger) dropCheckpoint(ctx context.Context, redisJobID string, crash CrashPoint) error {
	if crash == CrashAfterKeyDestroy {
		return ErrSimulatedCrash
	}
	return p.rdb.Del(ctx, memKey(redisJobID)).Err()
}

func (p *purger) signOutput(result *journal.JobResult) error {
	body := journal.BodyOf(result.Result)
	msg, err := seal.OutputMessage(result.ID, result.TenantID, body)
	if err != nil {
		return err
	}
	pub := p.priv.Public().(ed25519.PublicKey)
	result.Signature = seal.Sign(p.priv, msg)
	result.SigningKeyID = seal.SigningKeyID(pub)
	return nil
}

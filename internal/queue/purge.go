package queue

import (
	"context"
	"crypto/ed25519"
	"errors"
	"time"

	"coldharbour/internal/journal"
	"coldharbour/internal/redact"
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

func (p *purger) run(ctx context.Context, redisJobID string) error {
	durable := journal.DurableIDFor(redisJobID)
	if existing, ok, err := p.receipts.Get(ctx, durable); err != nil {
		return err
	} else if ok {
		if err := p.keys.Destroy(ctx, durable, existing.PurgedAt); err != nil {
			return err
		}
		return p.rdb.Del(ctx, memKey(redisJobID)).Err()
	}

	purgedAt := time.Now().UTC().Truncate(time.Microsecond).Truncate(time.Microsecond)
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
	return p.rdb.Del(ctx, memKey(redisJobID)).Err()
}

func (p *purger) signOutput(result *journal.JobResult) {
	body := redact.MarshalResult(result.Result)
	pub := p.priv.Public().(ed25519.PublicKey)
	result.Signature = seal.Sign(p.priv, body)
	result.SigningKeyID = seal.SigningKeyID(pub)
}

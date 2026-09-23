package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func EntryHash(prev []byte, kind string, payload []byte, at time.Time) []byte {
	h := sha256.New()
	h.Write(prev)
	h.Write([]byte(kind))
	h.Write(payload)
	h.Write([]byte(at.UTC().Format(time.RFC3339Nano)))
	sum := h.Sum(nil)
	return sum
}

func Zeros() []byte {
	return make([]byte, 32)
}

func canonicalPayload(raw []byte) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

func normalizeAt(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}

func Append(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, kind string, payload []byte, at time.Time) error {
	at = normalizeAt(at)
	payload = canonicalPayload(payload)
	if _, err := tx.Exec(ctx, `
		INSERT INTO ledger_heads (tenant_id, seq, hash)
		VALUES ($1, 0, $2)
		ON CONFLICT (tenant_id) DO NOTHING
	`, tenant, Zeros()); err != nil {
		return err
	}
	var seq int64
	var prev []byte
	if err := tx.QueryRow(ctx, `
		SELECT seq, hash FROM ledger_heads WHERE tenant_id = $1 FOR UPDATE
	`, tenant).Scan(&seq, &prev); err != nil {
		return err
	}
	if seq == 0 {
		prev = Zeros()
	}
	sum := EntryHash(prev, kind, payload, at)
	next := seq + 1
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_ledger (tenant_id, seq, prev_hash, entry_hash, kind, payload, at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, tenant, next, prev, sum, kind, payload, at); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE ledger_heads SET seq = $2, hash = $3 WHERE tenant_id = $1
	`, tenant, next, sum)
	return err
}

func AppendSQL(ctx context.Context, tx *sql.Tx, tenant uuid.UUID, kind string, payload []byte, at time.Time) error {
	at = normalizeAt(at)
	payload = canonicalPayload(payload)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_heads (tenant_id, seq, hash)
		VALUES ($1, 0, $2)
		ON CONFLICT (tenant_id) DO NOTHING
	`, tenant, Zeros()); err != nil {
		return err
	}
	var seq int64
	var prev []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT seq, hash FROM ledger_heads WHERE tenant_id = $1 FOR UPDATE
	`, tenant).Scan(&seq, &prev); err != nil {
		return err
	}
	if seq == 0 {
		prev = Zeros()
	}
	sum := EntryHash(prev, kind, payload, at)
	next := seq + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_ledger (tenant_id, seq, prev_hash, entry_hash, kind, payload, at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, tenant, next, prev, sum, kind, payload, at); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE ledger_heads SET seq = $2, hash = $3 WHERE tenant_id = $1
	`, tenant, next, sum)
	return err
}

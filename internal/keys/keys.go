package keys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"coldharbour/internal/ledger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Principal struct {
	TenantID uuid.UUID
	KeyID    uuid.UUID
	Role     string
}

type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func Authenticate(ctx context.Context, q Querier, header string) (Principal, bool) {
	prefix, ok := Prefix(header)
	if !ok {
		return Principal{}, false
	}
	var p Principal
	var hash []byte
	var revoked *time.Time
	err := q.QueryRow(ctx, `
		SELECT id, tenant_id, key_hash, role, revoked_at
		FROM api_keys WHERE prefix = $1
	`, prefix).Scan(&p.KeyID, &p.TenantID, &hash, &p.Role, &revoked)
	if err != nil || revoked != nil {
		return Principal{}, false
	}
	sum := sha256.Sum256([]byte(header))
	if subtle.ConstantTimeCompare(sum[:], hash) != 1 {
		return Principal{}, false
	}
	return p, true
}

func Prefix(full string) (string, bool) {
	parts := strings.SplitN(full, "_", 3)
	if len(parts) != 3 || parts[0] != "ch" || len(parts[1]) != 8 || parts[2] == "" {
		return "", false
	}
	return parts[1], true
}

func Create(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, name, role string) (id uuid.UUID, prefix, full string, err error) {
	prefix, err = newPrefix()
	if err != nil {
		return uuid.Nil, "", "", err
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return uuid.Nil, "", "", err
	}
	full = "ch_" + prefix + "_" + base64.RawURLEncoding.EncodeToString(secret)
	sum := sha256.Sum256([]byte(full))
	err = tx.QueryRow(ctx, `
		INSERT INTO api_keys (tenant_id, name, prefix, key_hash, role)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, tenant, name, prefix, sum[:], role).Scan(&id)
	if err != nil {
		return uuid.Nil, "", "", err
	}
	payload, err := json.Marshal(map[string]string{
		"id":     id.String(),
		"name":   name,
		"prefix": prefix,
		"role":   role,
	})
	if err != nil {
		return uuid.Nil, "", "", err
	}
	if err = ledger.Append(ctx, tx, tenant, "key_created", payload, time.Now()); err != nil {
		return uuid.Nil, "", "", err
	}
	return id, prefix, full, nil
}

func Revoke(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, `
		UPDATE api_keys SET revoked_at = now()
		WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL
	`, id, tenant)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	payload, err := json.Marshal(map[string]string{"id": id.String()})
	if err != nil {
		return err
	}
	return ledger.Append(ctx, tx, tenant, "key_revoked", payload, time.Now())
}

func newPrefix() (string, error) {
	raw := make([]byte, 5)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return enc.EncodeToString(raw), nil
}

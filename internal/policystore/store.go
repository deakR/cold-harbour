package policystore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"coldharbour/internal/ledger"
	"coldharbour/internal/policy"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type rowQuery interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func Load(ctx context.Context, q rowQuery, tenant uuid.UUID) (policy.Doc, bool, error) {
	var version int
	var raw []byte
	err := q.QueryRow(ctx, `SELECT version, document FROM tenant_policies WHERE tenant_id = $1`, tenant).Scan(&version, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return policy.Default(), false, nil
	}
	if err != nil {
		return policy.Doc{}, false, err
	}
	var doc policy.Doc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return policy.Doc{}, false, err
	}
	doc.Version = version
	return doc, true, nil
}

func Put(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, doc policy.Doc) (policy.Doc, error) {
	doc.Version = 0
	body, err := json.Marshal(doc)
	if err != nil {
		return policy.Doc{}, err
	}
	var version int
	if err := tx.QueryRow(ctx, `
		INSERT INTO tenant_policies (tenant_id, version, document)
		VALUES ($1, 1, $2)
		ON CONFLICT (tenant_id) DO UPDATE
		SET version = tenant_policies.version + 1,
		    document = EXCLUDED.document,
		    updated_at = now()
		RETURNING version
	`, tenant, body).Scan(&version); err != nil {
		return policy.Doc{}, err
	}
	doc.Version = version
	stored, err := json.Marshal(doc)
	if err != nil {
		return policy.Doc{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE tenant_policies SET document = $2 WHERE tenant_id = $1`, tenant, stored); err != nil {
		return policy.Doc{}, err
	}
	if err := ledger.Append(ctx, tx, tenant, "policy_changed", stored, time.Now()); err != nil {
		return policy.Doc{}, err
	}
	return doc, nil
}

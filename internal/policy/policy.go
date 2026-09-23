package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"coldharbour/internal/detect"
	"coldharbour/internal/ledger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Doc struct {
	Version        int           `json:"version"`
	Detectors      []detect.Kind `json:"detectors"`
	Mode           string        `json:"mode"`
	FailPolicy     string        `json:"failPolicy"`
	MaxInlineBytes int           `json:"maxInlineBytes"`
}

type FieldError struct {
	Field string
}

func (e *FieldError) Error() string { return e.Field }

func Default() Doc {
	return Doc{
		Version:        0,
		Detectors:      []detect.Kind{detect.KindEmail, detect.KindPhoneUS, detect.KindSSN},
		Mode:           string(detect.ModeRedact),
		FailPolicy:     "closed",
		MaxInlineBytes: 65536,
	}
}

var knownKinds = map[detect.Kind]struct{}{
	detect.KindEmail:   {},
	detect.KindPhoneUS: {},
	detect.KindPhoneIN: {},
	detect.KindSSN:     {},
	detect.KindAadhaar: {},
	detect.KindPAN:     {},
}

func Parse(raw []byte) (Doc, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc Doc
	if err := dec.Decode(&doc); err != nil {
		return Doc{}, &FieldError{Field: "detectors"}
	}
	if len(doc.Detectors) == 0 {
		return Doc{}, &FieldError{Field: "detectors"}
	}
	for _, kind := range doc.Detectors {
		if _, ok := knownKinds[kind]; !ok {
			return Doc{}, &FieldError{Field: "detectors"}
		}
	}
	if doc.Mode != string(detect.ModeRedact) && doc.Mode != string(detect.ModePartial) {
		return Doc{}, &FieldError{Field: "mode"}
	}
	if doc.FailPolicy != "closed" && doc.FailPolicy != "inline" && doc.FailPolicy != "open" {
		return Doc{}, &FieldError{Field: "failPolicy"}
	}
	if doc.MaxInlineBytes < 1 {
		return Doc{}, &FieldError{Field: "maxInlineBytes"}
	}
	return doc, nil
}

type rowQuery interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func Load(ctx context.Context, q rowQuery, tenant uuid.UUID) (Doc, bool, error) {
	var version int
	var raw []byte
	err := q.QueryRow(ctx, `SELECT version, document FROM tenant_policies WHERE tenant_id = $1`, tenant).Scan(&version, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Default(), false, nil
	}
	if err != nil {
		return Doc{}, false, err
	}
	var doc Doc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Doc{}, false, err
	}
	doc.Version = version
	return doc, true, nil
}

func Put(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, doc Doc) (Doc, error) {
	doc.Version = 0
	body, err := json.Marshal(doc)
	if err != nil {
		return Doc{}, err
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
		return Doc{}, err
	}
	doc.Version = version
	stored, err := json.Marshal(doc)
	if err != nil {
		return Doc{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE tenant_policies SET document = $2 WHERE tenant_id = $1`, tenant, stored); err != nil {
		return Doc{}, err
	}
	if err := ledger.Append(ctx, tx, tenant, "policy_changed", stored, time.Now()); err != nil {
		return Doc{}, err
	}
	return doc, nil
}

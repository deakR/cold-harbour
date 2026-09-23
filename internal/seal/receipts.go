package seal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"coldharbour/internal/ledger"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Receipt is one signed purge record keyed by DurableIDFor(redis id).
type Receipt struct {
	JobID        uuid.UUID
	RedisJobID   string
	PurgedAt     time.Time
	Signature    []byte
	SigningKeyID string
	TenantID     string
}

// ReceiptStore persists purge receipts. Insert is idempotent on JobID.
type ReceiptStore interface {
	Insert(ctx context.Context, r Receipt) (inserted bool, err error)
	Get(ctx context.Context, jobID uuid.UUID) (Receipt, bool, error)
}

// MemoryReceiptStore is an in-process store for tests.
type MemoryReceiptStore struct {
	mu   sync.Mutex
	rows map[uuid.UUID]Receipt
}

func NewMemoryReceiptStore() *MemoryReceiptStore {
	return &MemoryReceiptStore{rows: make(map[uuid.UUID]Receipt)}
}

func (s *MemoryReceiptStore) Insert(_ context.Context, r Receipt) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rows[r.JobID]; ok {
		return false, nil
	}
	cp := r
	cp.Signature = append([]byte(nil), r.Signature...)
	s.rows[r.JobID] = cp
	return true, nil
}

func (s *MemoryReceiptStore) Get(_ context.Context, jobID uuid.UUID) (Receipt, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[jobID]
	if !ok {
		return Receipt{}, false, nil
	}
	cp := r
	cp.Signature = append([]byte(nil), r.Signature...)
	return cp, true, nil
}

type pgReceiptStore struct {
	db *sql.DB
}

// OpenPostgresReceiptStore opens a ReceiptStore on the journal DSN.
func OpenPostgresReceiptStore(dsn string) (*pgReceiptStore, error) {
	if dsn == "" {
		return nil, errMissingDSN
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &pgReceiptStore{db: db}, nil
}

func (s *pgReceiptStore) Close() error {
	return s.db.Close()
}

func (s *pgReceiptStore) Insert(ctx context.Context, r Receipt) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO purge_receipts (job_id, redis_job_id, purged_at, signature, signing_key_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (job_id) DO NOTHING
	`, r.JobID, r.RedisJobID, r.PurgedAt, r.Signature, r.SigningKeyID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 1 && r.TenantID != "" {
		tenant, err := uuid.Parse(r.TenantID)
		if err != nil {
			return false, err
		}
		payload, err := json.Marshal(map[string]string{
			"jobId":        r.RedisJobID,
			"purgedAt":     r.PurgedAt.UTC().Format(time.RFC3339Nano),
			"signingKeyId": r.SigningKeyID,
		})
		if err != nil {
			return false, err
		}
		if err := ledger.AppendSQL(ctx, tx, tenant, "purge_receipt", payload, r.PurgedAt); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n == 1, nil
}

func (s *pgReceiptStore) Get(ctx context.Context, jobID uuid.UUID) (Receipt, bool, error) {
	var r Receipt
	err := s.db.QueryRowContext(ctx, `
		SELECT job_id, redis_job_id, purged_at, signature, signing_key_id
		FROM purge_receipts WHERE job_id = $1
	`, jobID).Scan(&r.JobID, &r.RedisJobID, &r.PurgedAt, &r.Signature, &r.SigningKeyID)
	if errors.Is(err, sql.ErrNoRows) {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, err
	}
	return r, true, nil
}

var (
	_ ReceiptStore = (*MemoryReceiptStore)(nil)
	_ ReceiptStore = (*pgReceiptStore)(nil)
)

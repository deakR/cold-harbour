package seal

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	errMissingDSN     = errors.New("POSTGRES_DSN is required")
	errKeyDestroyed   = errors.New("seal: job key material is nil")
	errKeyWrongLength = errors.New("seal: job key material must be 32 bytes")
)

// KeyStore holds per-job AES-256 material keyed by DurableIDFor(redisId).
type KeyStore interface {
	Ensure(ctx context.Context, durableID uuid.UUID) ([32]byte, error)
	Destroy(ctx context.Context, durableID uuid.UUID, at time.Time) error
}

type memoryKeyRow struct {
	material    [32]byte
	destroyed   bool
	destroyedAt time.Time
}

// MemoryKeyStore is an in-process store for tests.
type MemoryKeyStore struct {
	mu   sync.Mutex
	keys map[uuid.UUID]memoryKeyRow
}

func NewMemoryKeyStore() *MemoryKeyStore {
	return &MemoryKeyStore{keys: make(map[uuid.UUID]memoryKeyRow)}
}

func (s *MemoryKeyStore) Ensure(_ context.Context, durableID uuid.UUID) ([32]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if row, ok := s.keys[durableID]; ok {
		if row.destroyed {
			return [32]byte{}, errKeyDestroyed
		}
		return row.material, nil
	}
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return [32]byte{}, err
	}
	s.keys[durableID] = memoryKeyRow{material: key}
	return key, nil
}

func (s *MemoryKeyStore) Destroy(_ context.Context, durableID uuid.UUID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.keys[durableID]
	if !ok {
		return nil
	}
	if row.destroyed {
		return nil
	}
	row.destroyed = true
	row.destroyedAt = at.UTC()
	row.material = [32]byte{}
	s.keys[durableID] = row
	return nil
}

type pgKeyStore struct {
	db *sql.DB
}

// OpenPostgresKeyStore opens a KeyStore on the same DSN shape as the journal.
func OpenPostgresKeyStore(dsn string) (*pgKeyStore, error) {
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
	return &pgKeyStore{db: db}, nil
}

func (s *pgKeyStore) Close() error {
	return s.db.Close()
}

func (s *pgKeyStore) Ensure(ctx context.Context, durableID uuid.UUID) ([32]byte, error) {
	candidate := make([]byte, 32)
	if _, err := rand.Read(candidate); err != nil {
		return [32]byte{}, err
	}
	stored, err := wrapKey(ctx, candidate)
	if err != nil {
		return [32]byte{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO job_keys (job_id, key_material)
		VALUES ($1, $2)
		ON CONFLICT (job_id) DO NOTHING
	`, durableID, stored)
	if err != nil {
		return [32]byte{}, err
	}
	var material []byte
	err = s.db.QueryRowContext(ctx, `
		SELECT key_material FROM job_keys WHERE job_id = $1
	`, durableID).Scan(&material)
	if err != nil {
		return [32]byte{}, err
	}
	if material == nil {
		return [32]byte{}, errKeyDestroyed
	}
	plain, err := unwrapKey(ctx, material)
	if err != nil {
		return [32]byte{}, err
	}
	var key [32]byte
	copy(key[:], plain)
	return key, nil
}

func (s *pgKeyStore) Destroy(ctx context.Context, durableID uuid.UUID, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE job_keys
		SET key_material = NULL, destroyed_at = $2
		WHERE job_id = $1 AND key_material IS NOT NULL
	`, durableID, at.UTC())
	return err
}

var (
	_ KeyStore = (*MemoryKeyStore)(nil)
	_ KeyStore = (*pgKeyStore)(nil)
)

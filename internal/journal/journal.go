package journal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"coldharbour/internal/redact"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	errNotTerminal      = errors.New("journal records only COMPLETED or FAILED")
	errMissingPickup    = errors.New("terminal history has no CREATED to RUNNING pickup")
	errJournalConflict  = errors.New("durable row exists with a different checksum or final_state")
	ErrUnknownStoredJob = errors.New("no durable row for job")
	errMissingDSN       = errors.New("POSTGRES_DSN is required")
	errMissingTenant    = errors.New("tenant_id is required")
)

var jobNamespace = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("coldharbour.jobs"))

type Journal interface {
	Record(ctx context.Context, result JobResult) error
	Load(ctx context.Context, redisJobID string) (StoredJob, error)
}

type StoredJob struct {
	ID          uuid.UUID
	JobType     string
	FinalState  JobState
	Checksum    string
	CreatedAt   time.Time
	CompletedAt time.Time
}

type storedOutput struct {
	FinalState JobState
	Body       string
	TenantID   string
}

type MemoryJournal struct {
	mu      sync.Mutex
	rows    map[uuid.UUID]StoredJob
	outputs map[string]storedOutput
}

type pgJournal struct {
	db *sql.DB
}

func NewMemoryJournal() *MemoryJournal {
	return &MemoryJournal{
		rows:    make(map[uuid.UUID]StoredJob),
		outputs: make(map[string]storedOutput),
	}
}

func OpenJournal(dsn string) (*pgJournal, error) {
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
	return &pgJournal{db: db}, nil
}

func (j *pgJournal) Close() error {
	return j.db.Close()
}

func DurableIDFor(jobID string) uuid.UUID {
	return uuid.NewSHA1(jobNamespace, []byte(jobID))
}

// BodyOf returns the durable JSON body for a runner output map.
// Redact-shaped maps rematerialize through redact.MarshalResult so bytes match
// the typed encoder (map key order from encoding/json would otherwise diverge).
func BodyOf(result map[string]any) []byte {
	b, err := json.Marshal(result)
	if err != nil {
		panic(err)
	}
	if _, ok := result["redactedText"]; !ok {
		return b
	}
	var rr redact.RedactResult
	if err := json.Unmarshal(b, &rr); err != nil {
		return b
	}
	return redact.MarshalResult(rr)
}

func ChecksumOf(result map[string]any) string {
	sum := sha256.Sum256(BodyOf(result))
	return hex.EncodeToString(sum[:])
}

func parseTerminal(result JobResult) (StoredJob, error) {
	steps := result.History.Transitions()
	if len(steps) == 0 {
		return StoredJob{}, errNotTerminal
	}
	last := steps[len(steps)-1]
	if last.To != COMPLETED && last.To != FAILED {
		return StoredJob{}, errNotTerminal
	}
	var created time.Time
	foundPickup := false
	for _, step := range steps {
		if step.From == CREATED && step.To == RUNNING {
			created = step.At
			foundPickup = true
			break
		}
	}
	if !foundPickup {
		return StoredJob{}, errMissingPickup
	}
	jobType := result.JobType
	if jobType == "" {
		jobType = "redact"
	}
	return StoredJob{
		ID:          DurableIDFor(result.ID),
		JobType:     jobType,
		FinalState:  last.To,
		Checksum:    ChecksumOf(result.Result),
		CreatedAt:   created,
		CompletedAt: last.At,
	}, nil
}

func acceptExisting(existing, incoming StoredJob) error {
	if existing.Checksum == incoming.Checksum && existing.FinalState == incoming.FinalState {
		return nil
	}
	return errJournalConflict
}

func (j *MemoryJournal) Record(_ context.Context, result JobResult) error {
	row, err := parseTerminal(result)
	if err != nil {
		return err
	}
	body := string(BodyOf(result.Result))
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.rows == nil {
		j.rows = make(map[uuid.UUID]StoredJob)
	}
	if j.outputs == nil {
		j.outputs = make(map[string]storedOutput)
	}
	existing, ok := j.rows[row.ID]
	if !ok {
		j.outputs[result.ID] = storedOutput{FinalState: row.FinalState, Body: body, TenantID: result.TenantID}
		j.rows[row.ID] = row
		return nil
	}
	return acceptExisting(existing, row)
}

func (j *MemoryJournal) Load(_ context.Context, redisJobID string) (StoredJob, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	row, ok := j.rows[DurableIDFor(redisJobID)]
	if !ok {
		return StoredJob{}, ErrUnknownStoredJob
	}
	return row, nil
}

func (j *MemoryJournal) LoadOutput(_ context.Context, redisJobID string) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	out, ok := j.outputs[redisJobID]
	if !ok {
		return "", ErrUnknownStoredJob
	}
	return out.Body, nil
}

func (j *pgJournal) Record(ctx context.Context, result JobResult) error {
	if result.TenantID == "" {
		return errMissingTenant
	}
	row, err := parseTerminal(result)
	if err != nil {
		return err
	}
	body := string(BodyOf(result.Result))
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outputs (redis_job_id, final_state, body, tenant_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (redis_job_id) DO NOTHING
	`, result.ID, string(row.FinalState), body, result.TenantID); err != nil {
		return err
	}

	var sig any
	if len(result.Signature) > 0 {
		sig = result.Signature
	}
	var signingKeyID any
	if result.SigningKeyID != "" {
		signingKeyID = result.SigningKeyID
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO jobs (id, job_type, final_state, output_checksum, created_at, completed_at, tenant_id, output_signature, signing_key_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO NOTHING
	`, row.ID, row.JobType, string(row.FinalState), row.Checksum, row.CreatedAt, row.CompletedAt, result.TenantID, sig, signingKeyID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return tx.Commit()
	}
	var checksum, finalState string
	err = tx.QueryRowContext(ctx, `
		SELECT output_checksum, final_state FROM jobs WHERE id = $1
	`, row.ID).Scan(&checksum, &finalState)
	if err != nil {
		return err
	}
	if err := acceptExisting(StoredJob{Checksum: checksum, FinalState: JobState(finalState)}, row); err != nil {
		return err
	}
	return tx.Commit()
}

func (j *pgJournal) Load(ctx context.Context, redisJobID string) (StoredJob, error) {
	id := DurableIDFor(redisJobID)
	var row StoredJob
	var finalState string
	err := j.db.QueryRowContext(ctx, `
		SELECT id, job_type, final_state, output_checksum, created_at, completed_at
		FROM jobs WHERE id = $1
	`, id).Scan(&row.ID, &row.JobType, &finalState, &row.Checksum, &row.CreatedAt, &row.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredJob{}, ErrUnknownStoredJob
	}
	if err != nil {
		return StoredJob{}, err
	}
	row.FinalState = JobState(finalState)
	return row, nil
}

var (
	_ Journal = (*MemoryJournal)(nil)
	_ Journal = (*pgJournal)(nil)
)

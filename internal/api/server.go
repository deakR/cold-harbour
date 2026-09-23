package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"coldharbour/internal/queue"
	"coldharbour/internal/seal"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type enqueuer interface {
	Add(ctx context.Context, keys seal.KeyStore, job queue.Job) error
}

type Server struct {
	db    *pgxpool.Pool
	rdb   *redis.Client
	keys  seal.KeyStore
	jobs  enqueuer
	types map[string]struct{}
	hub   *hub
	now   func() time.Time
}

func New(db *pgxpool.Pool, rdb *redis.Client, keys seal.KeyStore, jobs enqueuer, types []string) *Server {
	set := make(map[string]struct{}, len(types))
	for _, name := range types {
		set[name] = struct{}{}
	}
	s := &Server{
		db:    db,
		rdb:   rdb,
		keys:  keys,
		jobs:  jobs,
		types: set,
		hub:   newHub(),
		now:   time.Now,
	}
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/jobs", s.authed(s.postJob))
	mux.HandleFunc("GET /v1/jobs", s.authed(s.listJobs))
	mux.HandleFunc("GET /v1/jobs/{id}", s.authed(s.getJob))
	mux.HandleFunc("POST /v1/jobs/{id}/delivery-links", s.authed(s.createLink))
	mux.HandleFunc("GET /v1/reports/compliance", s.authed(s.compliance))
	mux.HandleFunc("GET /v1/ws/events", s.events)
	mux.HandleFunc("GET /d/{token}", s.openLink)
	return mux
}

func (s *Server) authed(next func(http.ResponseWriter, *http.Request, uuid.UUID)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := s.authenticate(r)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next(w, r, tenant)
	}
}

func (s *Server) authenticate(r *http.Request) (uuid.UUID, bool) {
	raw := r.Header.Get("X-API-Key")
	if raw == "" && strings.HasPrefix(r.URL.Path, "/v1/ws/") {
		raw = r.URL.Query().Get("apiKey")
	}
	if raw == "" {
		return uuid.Nil, false
	}
	sum := sha256.Sum256([]byte(raw))
	var tenant uuid.UUID
	err := s.db.QueryRow(r.Context(), `
		SELECT tenant_id FROM api_keys
		WHERE key_hash = $1 AND revoked_at IS NULL
	`, hex.EncodeToString(sum[:])).Scan(&tenant)
	if err != nil {
		return uuid.Nil, false
	}
	return tenant, true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func fieldError(w http.ResponseWriter, field string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"field": field})
}

func readJSON(r *http.Request) (map[string]string, error) {
	if r.Body == nil {
		return map[string]string{}, nil
	}
	defer r.Body.Close()
	var body map[string]string
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if body == nil {
		body = map[string]string{}
	}
	return body, nil
}

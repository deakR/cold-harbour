package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"coldharbour/internal/keys"
	"coldharbour/internal/queue"
	"coldharbour/internal/seal"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type enqueuer interface {
	Add(ctx context.Context, keys seal.KeyStore, job queue.Job) error
}

type Server struct {
	db              *pgxpool.Pool
	rdb             *redis.Client
	keys            seal.KeyStore
	jobs            enqueuer
	types           map[string]struct{}
	hub             *hub
	now             func() time.Time
	wsOrigins       []string
	insecureCookies bool
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

func (s *Server) SetWSOrigins(patterns []string) {
	s.wsOrigins = append([]string(nil), patterns...)
}

func (s *Server) SetInsecureCookies(v bool) {
	s.insecureCookies = v
}

func ParseWSOrigins(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"localhost:5173"}
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{"localhost:5173"}
	}
	return out
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/jobs", s.authed([]string{"admin", "app", "sentinel"}, s.postJob))
	mux.HandleFunc("GET /v1/jobs", s.authed([]string{"admin", "app"}, s.listJobs))
	mux.HandleFunc("GET /v1/jobs/{id}", s.authed([]string{"admin", "app"}, s.getJob))
	mux.HandleFunc("POST /v1/jobs/{id}/delivery-links", s.authed([]string{"admin", "app"}, s.createLink))
	mux.HandleFunc("GET /v1/reports/compliance", s.authed([]string{"admin", "app"}, s.compliance))
	mux.HandleFunc("GET /v1/policy", s.authed([]string{"admin", "app", "sentinel"}, s.getPolicy))
	mux.HandleFunc("PUT /v1/policy", s.authed([]string{"admin"}, s.putPolicy))
	mux.HandleFunc("POST /v1/keys", s.authed([]string{"admin"}, s.createKey))
	mux.HandleFunc("GET /v1/keys", s.authed([]string{"admin"}, s.listKeys))
	mux.HandleFunc("DELETE /v1/keys/{id}", s.authed([]string{"admin"}, s.revokeKey))
	mux.HandleFunc("POST /v1/sentinel/events", s.authed([]string{"sentinel"}, s.postSentinelEvents))
	mux.HandleFunc("GET /v1/sentinel/nodes", s.authed([]string{"admin", "app", "sentinel"}, s.listSentinelNodes))
	mux.HandleFunc("GET /v1/ws/events", s.events)
	mux.HandleFunc("GET /d/{token}", s.openLink)
	return mux
}

func (s *Server) authed(roles []string, next func(http.ResponseWriter, *http.Request, keys.Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.authenticate(r)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !roleAllowed(p.Role, roles) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r, p)
	}
}

func roleAllowed(role string, roles []string) bool {
	for _, allowed := range roles {
		if role == allowed {
			return true
		}
	}
	return false
}

func (s *Server) authenticate(r *http.Request) (keys.Principal, bool) {
	raw := r.Header.Get("X-API-Key")
	if raw == "" && strings.HasPrefix(r.URL.Path, "/v1/ws/") {
		raw = r.URL.Query().Get("apiKey")
	}
	return keys.Authenticate(r.Context(), s.db, raw)
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

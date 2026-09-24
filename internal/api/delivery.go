package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"coldharbour/internal/journal"
	"coldharbour/internal/keys"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) createLink(w http.ResponseWriter, r *http.Request, p keys.Principal) {
	tenant := p.TenantID
	id := r.PathValue("id")
	var owned int
	err := s.db.QueryRow(r.Context(), `
		SELECT 1 FROM job_accepts WHERE redis_job_id = $1 AND tenant_id = $2
	`, id, tenant).Scan(&owned)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	body, err := readJSON(r)
	if err != nil {
		fieldError(w, "expiresAt")
		return
	}
	expiresAt, ok := s.parseExpiry(body["expiresAt"])
	if !ok {
		fieldError(w, "expiresAt")
		return
	}
	views, ok := parseViews(body["maxViews"])
	if !ok {
		fieldError(w, "maxViews")
		return
	}
	var ready int
	err = s.db.QueryRow(r.Context(), `
		SELECT 1 FROM outputs WHERE redis_job_id = $1 AND tenant_id = $2
	`, id, tenant).Scan(&ready)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "job has no output"})
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	token := hex.EncodeToString(raw)
	_, err = s.db.Exec(r.Context(), `
		INSERT INTO delivery_links (token_hash, redis_job_id, tenant_id, expires_at, max_views)
		VALUES ($1, $2, $3, $4, $5)
	`, sha256Hex(token), id, tenant, expiresAt, views)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":     token,
		"expiresAt": body["expiresAt"],
		"maxViews":  views,
	})
}

func (s *Server) parseExpiry(raw string) (time.Time, bool) {
	at, ok := parseInstant(raw)
	if !ok {
		return time.Time{}, false
	}
	now := s.now()
	if !at.After(now) || at.After(now.Add(30*24*time.Hour)) {
		return time.Time{}, false
	}
	return at, true
}

func parseViews(raw string) (int, bool) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100 {
		return 0, false
	}
	return n, true
}

func sha256Hex(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Server) openLink(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	hash := sha256Hex(token)
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	defer tx.Rollback(r.Context())

	var jobID string
	var linkTenant uuid.UUID
	var expires time.Time
	var maxViews, views int
	err = tx.QueryRow(r.Context(), `
		SELECT redis_job_id, tenant_id, expires_at, max_views, view_count
		FROM delivery_links
		WHERE token_hash = $1
		FOR UPDATE
	`, hash).Scan(&jobID, &linkTenant, &expires, &maxViews, &views)
	if err == pgx.ErrNoRows {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	if !expires.After(s.now()) {
		w.WriteHeader(http.StatusGone)
		return
	}
	if views >= maxViews {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	var body, tenantID, keyID string
	var sig []byte
	err = tx.QueryRow(r.Context(), `
		SELECT o.body, o.tenant_id::text, j.output_signature, COALESCE(j.signing_key_id, '')
		FROM outputs o
		JOIN jobs j ON j.tenant_id = o.tenant_id AND j.id = $1
		WHERE o.redis_job_id = $2 AND o.tenant_id = $3
	`, journal.DurableIDFor(jobID), jobID, linkTenant).Scan(&body, &tenantID, &sig, &keyID)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "job has no output"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE delivery_links SET view_count = view_count + 1 WHERE token_hash = $1
	`, hash)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO delivery_link_accesses (token_hash) VALUES ($1)
	`, hash)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"result":       jsonRaw(body),
		"tenantId":     tenantID,
		"signature":    b64(sig),
		"signingKeyId": keyID,
	})
}

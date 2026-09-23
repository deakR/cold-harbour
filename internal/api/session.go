package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"coldharbour/internal/keys"
	"coldharbour/internal/ledger"

	"github.com/google/uuid"
)

const (
	sessionIdle     = 30 * time.Minute
	sessionAbsolute = 8 * time.Hour
	cookieHostName  = "__Host-ch_session"
	cookieDevName   = "ch_session"
)

type ctxKey int

const sessionCSRFKey ctxKey = 1

func (s *Server) sessionCookieName() string {
	if s.insecureCookies {
		return cookieDevName
	}
	return cookieHostName
}

func (s *Server) applyInsecureCookieEnv() {
	if os.Getenv("ALLOW_INSECURE_SESSION_COOKIE") == "1" {
		s.insecureCookies = true
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	body, err := readJSON(r)
	if err != nil {
		fieldError(w, "apiKey")
		return
	}
	raw := body["apiKey"]
	p, ok := keys.Authenticate(r.Context(), s.db, raw)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if p.Role != "admin" && p.Role != "app" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	token, err := randomToken()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	csrf, err := randomToken()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	now := s.now()
	expires := now.Add(sessionAbsolute)
	sum := sha256.Sum256([]byte(token))
	sessionID := uuid.New()

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(), `
		INSERT INTO sessions (id, tenant_id, key_id, token_hash, csrf_token, last_seen_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, sessionID, p.TenantID, p.KeyID, sum[:], csrf, now, expires)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	payload, err := json.Marshal(map[string]string{
		"sessionId": sessionID.String(),
		"keyId":     p.KeyID.String(),
		"expiresAt": expires.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	if err := ledger.Append(r.Context(), tx, p.TenantID, "session_created", payload, now); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}

	http.SetCookie(w, s.sessionCookie(token, expires))
	writeJSON(w, http.StatusOK, map[string]string{"csrfToken": csrf})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, p keys.Principal) {
	c, err := r.Cookie(s.sessionCookieName())
	if err != nil || c.Value == "" {
		s.clearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	sum := sha256.Sum256([]byte(c.Value))
	now := s.now()
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	defer tx.Rollback(r.Context())

	var sessionID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		UPDATE sessions
		SET revoked_at = $1
		WHERE token_hash = $2 AND tenant_id = $3 AND revoked_at IS NULL
		RETURNING id
	`, now, sum[:], p.TenantID).Scan(&sessionID)
	if err != nil {
		s.clearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	payload, err := json.Marshal(map[string]string{
		"sessionId": sessionID.String(),
		"keyId":     p.KeyID.String(),
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	if err := ledger.Append(r.Context(), tx, p.TenantID, "session_revoked", payload, now); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request, _ keys.Principal) {
	csrf, ok := r.Context().Value(sessionCSRFKey).(string)
	if !ok || csrf == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"csrfToken": csrf})
}

func (s *Server) authenticate(r *http.Request) (keys.Principal, bool, string, bool) {
	raw := r.Header.Get("X-API-Key")
	if raw != "" {
		p, ok := keys.Authenticate(r.Context(), s.db, raw)
		return p, false, "", ok
	}
	c, err := r.Cookie(s.sessionCookieName())
	if err != nil || c.Value == "" {
		return keys.Principal{}, false, "", false
	}
	sum := sha256.Sum256([]byte(c.Value))
	now := s.now()
	var (
		p         keys.Principal
		csrf      string
		sessionID uuid.UUID
		lastSeen  time.Time
		expires   time.Time
		revoked   *time.Time
		keyRev    *time.Time
	)
	err = s.db.QueryRow(r.Context(), `
		SELECT s.id, s.tenant_id, s.key_id, k.role, s.csrf_token, s.last_seen_at, s.expires_at, s.revoked_at, k.revoked_at
		FROM sessions s
		JOIN api_keys k ON k.id = s.key_id
		WHERE s.token_hash = $1
	`, sum[:]).Scan(&sessionID, &p.TenantID, &p.KeyID, &p.Role, &csrf, &lastSeen, &expires, &revoked, &keyRev)
	if err != nil || revoked != nil || keyRev != nil {
		return keys.Principal{}, false, "", false
	}
	if !expires.After(now) {
		return keys.Principal{}, false, "", false
	}
	if now.Sub(lastSeen) > sessionIdle {
		return keys.Principal{}, false, "", false
	}
	_, _ = s.db.Exec(r.Context(), `
		UPDATE sessions SET last_seen_at = $1 WHERE id = $2 AND revoked_at IS NULL
	`, now, sessionID)
	return p, true, csrf, true
}

func (s *Server) requireCookieCSRF(r *http.Request, csrf string) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return true
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) != 1 {
		return false
	}
	return s.originAllowed(r.Header.Get("Origin"))
}

func (s *Server) originAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	patterns := s.wsOrigins
	if len(patterns) == 0 {
		patterns = []string{"localhost:5173"}
	}
	host := u.Host
	for _, p := range patterns {
		if host == p || matchOriginPattern(host, p) {
			return true
		}
	}
	return false
}

func matchOriginPattern(host, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return host == pattern
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:]
		return strings.HasSuffix(host, suffix) || host == pattern[2:]
	}
	return false
}

func (s *Server) sessionCookie(token string, expires time.Time) *http.Cookie {
	c := &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   !s.insecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	}
	return c
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	c := &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   !s.insecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
	http.SetCookie(w, c)
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

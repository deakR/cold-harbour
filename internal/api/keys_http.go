package api

import (
	"net/http"
	"time"

	"coldharbour/internal/keys"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) createKey(w http.ResponseWriter, r *http.Request, p keys.Principal) {
	body, err := readJSON(r)
	if err != nil {
		fieldError(w, "name")
		return
	}
	name := body["name"]
	role := body["role"]
	if name == "" {
		fieldError(w, "name")
		return
	}
	if role != "admin" && role != "app" && role != "sentinel" {
		fieldError(w, "role")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	id, prefix, full, err := keys.Create(r.Context(), tx, p.TenantID, name, role)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"id":     id.String(),
		"prefix": prefix,
		"key":    full,
	})
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request, p keys.Principal) {
	rows, err := s.db.Query(r.Context(), `
		SELECT id, name, prefix, role, created_at, revoked_at
		FROM api_keys WHERE tenant_id = $1
		ORDER BY created_at
	`, p.TenantID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	defer rows.Close()
	type item struct {
		ID        string     `json:"id"`
		Name      string     `json:"name"`
		Prefix    string     `json:"prefix"`
		Role      string     `json:"role"`
		CreatedAt time.Time  `json:"createdAt"`
		RevokedAt *time.Time `json:"revokedAt"`
	}
	out := []item{}
	for rows.Next() {
		var row item
		var id uuid.UUID
		if err := rows.Scan(&id, &row.Name, &row.Prefix, &row.Role, &row.CreatedAt, &row.RevokedAt); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
			return
		}
		row.ID = id.String()
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request, p keys.Principal) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	if err := keys.Revoke(r.Context(), tx, p.TenantID, id); err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

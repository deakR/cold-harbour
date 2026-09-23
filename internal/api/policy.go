package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"coldharbour/internal/keys"
	"coldharbour/internal/policy"
)

func (s *Server) getPolicy(w http.ResponseWriter, r *http.Request, p keys.Principal) {
	doc, _, err := policy.Load(r.Context(), s.db, p.TenantID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "policy unavailable"})
		return
	}
	etag := strconv.Quote(strconv.Itoa(doc.Version))
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) putPolicy(w http.ResponseWriter, r *http.Request, p keys.Principal) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		fieldError(w, "detectors")
		return
	}
	doc, err := policy.Parse(raw)
	if err != nil {
		var field *policy.FieldError
		if errors.As(err, &field) {
			fieldError(w, field.Field)
			return
		}
		fieldError(w, "detectors")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "policy unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	stored, err := policy.Put(r.Context(), tx, p.TenantID, doc)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "policy unavailable"})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "policy unavailable"})
		return
	}
	w.Header().Set("ETag", strconv.Quote(strconv.Itoa(stored.Version)))
	writeJSON(w, http.StatusOK, stored)
}

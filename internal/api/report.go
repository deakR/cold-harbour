package api

import (
	"net/http"
	"time"

	"coldharbour/internal/journal"
	"coldharbour/internal/keys"
)

func (s *Server) compliance(w http.ResponseWriter, r *http.Request, p keys.Principal) {
	tenant := p.TenantID
	from, ok := parseInstant(r.URL.Query().Get("from"))
	if !ok {
		fieldError(w, "from")
		return
	}
	to, ok := parseInstant(r.URL.Query().Get("to"))
	if !ok {
		fieldError(w, "to")
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT redis_job_id, accepted_at
		FROM job_accepts
		WHERE tenant_id = $1 AND accepted_at >= $2 AND accepted_at < $3
		ORDER BY accepted_at
	`, tenant, from, to)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	defer rows.Close()
	csv := "dispatch_time,completion_time,final_state,checksum,signature_status,purge_timestamp\n"
	for rows.Next() {
		var redisID string
		var accepted time.Time
		if err := rows.Scan(&redisID, &accepted); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
			return
		}
		durable := journal.DurableIDFor(redisID)
		var completed *time.Time
		var state, checksum *string
		var sig []byte
		err = s.db.QueryRow(r.Context(), `
			SELECT completed_at, final_state, output_checksum, output_signature
			FROM jobs WHERE id = $1 AND tenant_id = $2
		`, durable, tenant).Scan(&completed, &state, &checksum, &sig)
		completedText := ""
		stateText := ""
		checksumText := ""
		signatureStatus := "absent"
		if err == nil {
			if completed != nil {
				completedText = completed.UTC().Format(time.RFC3339Nano)
			}
			if state != nil {
				stateText = *state
			}
			if checksum != nil {
				checksumText = *checksum
			}
			if len(sig) > 0 {
				signatureStatus = "present"
			}
		}
		var purged *time.Time
		_ = s.db.QueryRow(r.Context(), `
			SELECT purged_at FROM purge_receipts WHERE job_id = $1
		`, durable).Scan(&purged)
		purgedText := ""
		if purged != nil {
			purgedText = purged.UTC().Format(time.RFC3339Nano)
		}
		csv += accepted.UTC().Format(time.RFC3339Nano) + "," + completedText + "," + stateText + "," + checksumText + "," + signatureStatus + "," + purgedText + "\n"
	}
	w.Header().Set("Content-Type", "text/csv")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(csv))
}

func parseInstant(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		at, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, false
		}
	}
	return at, true
}

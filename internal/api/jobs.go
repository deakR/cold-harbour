package api

import (
	"net/http"
	"strconv"
	"time"

	"coldharbour/internal/journal"
	"coldharbour/internal/queue"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) postJob(w http.ResponseWriter, r *http.Request, tenant uuid.UUID) {
	body, err := readJSON(r)
	if err != nil {
		fieldError(w, "input")
		return
	}
	input := body["input"]
	if input == "" {
		fieldError(w, "input")
		return
	}
	jobType := body["jobType"]
	if jobType == "" {
		jobType = "redact"
	}
	if _, ok := s.types[jobType]; !ok {
		fieldError(w, "jobType")
		return
	}
	allowed, err := s.allowPost(r, tenant)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	if !allowed {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	jobID := uuid.NewString()
	_, err = s.db.Exec(r.Context(), `
		INSERT INTO job_accepts (redis_job_id, tenant_id, accepted_at)
		VALUES ($1, $2, now())
	`, jobID, tenant)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	err = s.jobs.Add(r.Context(), s.keys, queue.Job{
		ID:       jobID,
		Input:    input,
		TenantID: tenant.String(),
		JobType:  jobType,
	})
	if err != nil {
		_, _ = s.db.Exec(r.Context(), `DELETE FROM job_keys WHERE job_id = $1`, journal.DurableIDFor(jobID))
		_, _ = s.db.Exec(r.Context(), `DELETE FROM job_accepts WHERE redis_job_id = $1`, jobID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"jobId": jobID, "status": "QUEUED"})
}

func (s *Server) allowPost(r *http.Request, tenant uuid.UUID) (bool, error) {
	window := s.now().Unix() / 60
	key := "ratelimit:post:" + tenant.String() + ":" + strconv.FormatInt(window, 10)
	n, err := s.rdb.Incr(r.Context(), key).Result()
	if err != nil {
		return false, err
	}
	if n == 1 {
		_ = s.rdb.Expire(r.Context(), key, 120*time.Second).Err()
	}
	return n <= 30, nil
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request, tenant uuid.UUID) {
	rows, err := s.db.Query(r.Context(), `
		SELECT a.redis_job_id, a.accepted_at, o.final_state
		FROM job_accepts a
		LEFT JOIN outputs o ON o.redis_job_id = a.redis_job_id AND o.tenant_id = a.tenant_id
		WHERE a.tenant_id = $1
		ORDER BY a.accepted_at DESC
	`, tenant)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	defer rows.Close()
	type row struct {
		JobID      string `json:"jobId"`
		AcceptedAt string `json:"acceptedAt"`
		Status     string `json:"status"`
	}
	out := []row{}
	for rows.Next() {
		var item row
		var accepted time.Time
		var state *string
		if err := rows.Scan(&item.JobID, &accepted, &state); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
			return
		}
		item.AcceptedAt = accepted.UTC().Format(time.RFC3339Nano)
		item.Status = "QUEUED"
		if state != nil && *state != "" {
			item.Status = *state
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request, tenant uuid.UUID) {
	id := r.PathValue("id")
	var state, body, keyID string
	var sig []byte
	err := s.db.QueryRow(r.Context(), `
		SELECT o.final_state, o.body, j.output_signature, COALESCE(j.signing_key_id, '')
		FROM outputs o
		LEFT JOIN jobs j ON j.id = $1 AND j.tenant_id = o.tenant_id
		WHERE o.redis_job_id = $2 AND o.tenant_id = $3
	`, journal.DurableIDFor(id), id, tenant).Scan(&state, &body, &sig, &keyID)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"jobId":        id,
			"status":       state,
			"result":       jsonRaw(body),
			"tenantId":     tenant.String(),
			"signature":    b64(sig),
			"signingKeyId": keyID,
		})
		return
	}
	if err != nil && err != pgx.ErrNoRows {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	var one int
	err = s.db.QueryRow(r.Context(), `
		SELECT 1 FROM job_accepts WHERE redis_job_id = $1 AND tenant_id = $2
	`, id, tenant).Scan(&one)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	running, err := queue.JobRunning(r.Context(), s.rdb, id)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue unavailable"})
		return
	}
	status := "QUEUED"
	if running {
		status = "RUNNING"
	}
	writeJSON(w, http.StatusOK, map[string]string{"jobId": id, "status": status})
}

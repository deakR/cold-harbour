package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"coldharbour/internal/keys"
	"coldharbour/internal/ledger"
)

type sentinelBatch struct {
	NodeID        string         `json:"nodeId"`
	From          time.Time      `json:"from"`
	To            time.Time      `json:"to"`
	PolicyVersion int            `json:"policyVersion"`
	Counts        map[string]int `json:"counts"`
	Held          int64          `json:"held"`
	Dropped       int64          `json:"dropped"`
	FailOpen      int64          `json:"failOpen"`
}

type sentinelNodeRow struct {
	NodeID        string         `json:"nodeId"`
	LastSeen      string         `json:"lastSeen"`
	PolicyVersion int            `json:"policyVersion"`
	Counts        map[string]int `json:"counts"`
	Held          int64          `json:"held"`
	Dropped       int64          `json:"dropped"`
	FailOpen      int64          `json:"failOpen"`
}

func (s *Server) postSentinelEvents(w http.ResponseWriter, r *http.Request, p keys.Principal) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		fieldError(w, "nodeId")
		return
	}
	batch, field, ok := parseSentinelBatch(raw)
	if !ok {
		fieldError(w, field)
		return
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		fieldError(w, "nodeId")
		return
	}
	countsJSON, err := json.Marshal(batch.Counts)
	if err != nil {
		fieldError(w, "counts")
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `
		INSERT INTO sentinel_nodes (tenant_id, node_id, last_seen, policy_version, counts, held, dropped, fail_open)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tenant_id, node_id) DO UPDATE
		SET last_seen = EXCLUDED.last_seen,
		    policy_version = EXCLUDED.policy_version,
		    counts = EXCLUDED.counts,
		    held = EXCLUDED.held,
		    dropped = EXCLUDED.dropped,
		    fail_open = EXCLUDED.fail_open
	`, p.TenantID, batch.NodeID, batch.To, batch.PolicyVersion, countsJSON, batch.Held, batch.Dropped, batch.FailOpen)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	if err := ledger.Append(r.Context(), tx, p.TenantID, "sentinel_batch", payload, time.Now()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listSentinelNodes(w http.ResponseWriter, r *http.Request, p keys.Principal) {
	rows, err := s.db.Query(r.Context(), `
		SELECT node_id, last_seen, policy_version, counts, held, dropped, fail_open
		FROM sentinel_nodes
		WHERE tenant_id = $1
		ORDER BY last_seen DESC
	`, p.TenantID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	defer rows.Close()
	out := []sentinelNodeRow{}
	for rows.Next() {
		var item sentinelNodeRow
		var lastSeen time.Time
		var countsRaw []byte
		if err := rows.Scan(&item.NodeID, &lastSeen, &item.PolicyVersion, &countsRaw, &item.Held, &item.Dropped, &item.FailOpen); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
			return
		}
		item.LastSeen = lastSeen.UTC().Format(time.RFC3339Nano)
		item.Counts = map[string]int{}
		if len(countsRaw) > 0 {
			_ = json.Unmarshal(countsRaw, &item.Counts)
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

func parseSentinelBatch(raw []byte) (sentinelBatch, string, bool) {
	var wire struct {
		NodeID        *string         `json:"nodeId"`
		From          *string         `json:"from"`
		To            *string         `json:"to"`
		PolicyVersion *int            `json:"policyVersion"`
		Counts        *map[string]int `json:"counts"`
		Held          *int64          `json:"held"`
		Dropped       *int64          `json:"dropped"`
		FailOpen      *int64          `json:"failOpen"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return sentinelBatch{}, "nodeId", false
	}
	if wire.NodeID == nil || *wire.NodeID == "" {
		return sentinelBatch{}, "nodeId", false
	}
	if wire.From == nil {
		return sentinelBatch{}, "from", false
	}
	from, ok := parseInstant(*wire.From)
	if !ok {
		return sentinelBatch{}, "from", false
	}
	if wire.To == nil {
		return sentinelBatch{}, "to", false
	}
	to, ok := parseInstant(*wire.To)
	if !ok {
		return sentinelBatch{}, "to", false
	}
	if !to.After(from) && !to.Equal(from) {
		return sentinelBatch{}, "to", false
	}
	if wire.PolicyVersion == nil || *wire.PolicyVersion < 0 {
		return sentinelBatch{}, "policyVersion", false
	}
	if wire.Counts == nil {
		return sentinelBatch{}, "counts", false
	}
	for k, n := range *wire.Counts {
		if k == "" || n < 0 {
			return sentinelBatch{}, "counts", false
		}
	}
	if wire.Held == nil || *wire.Held < 0 {
		return sentinelBatch{}, "held", false
	}
	if wire.Dropped == nil || *wire.Dropped < 0 {
		return sentinelBatch{}, "dropped", false
	}
	if wire.FailOpen == nil || *wire.FailOpen < 0 {
		return sentinelBatch{}, "failOpen", false
	}
	counts := make(map[string]int, len(*wire.Counts))
	for k, n := range *wire.Counts {
		counts[k] = n
	}
	return sentinelBatch{
		NodeID:        *wire.NodeID,
		From:          from,
		To:            to,
		PolicyVersion: *wire.PolicyVersion,
		Counts:        counts,
		Held:          *wire.Held,
		Dropped:       *wire.Dropped,
		FailOpen:      *wire.FailOpen,
	}, "", true
}

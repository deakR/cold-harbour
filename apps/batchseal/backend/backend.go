// Package backend is the BatchSeal desktop client for the ColdHarbor control
// plane. Standard library only: REST over net/http with polling (no websocket
// dependency), SHA-256 seal verification with crypto/sha256, and local receipt
// storage under the OS user config directory.
package backend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config holds connection settings, persisted to disk.
type Config struct {
	BaseURL   string `json:"baseURL"`
	APIKey    string `json:"apiKey"`
	Clearance string `json:"clearance"`
	OwnerID   string `json:"ownerID"`
}

// DefaultConfig returns local-dev defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "http://localhost:8080",
		Clearance: "INNIE",
		OwnerID:   "usr_batchseal",
	}
}

// Compartment is the lifecycle state of one batch.
type Compartment struct {
	CompartmentID string `json:"compartmentId"`
	Context       string `json:"context"`
	OwnerID       string `json:"ownerId"`
	TaskType      string `json:"taskType"`
	State         string `json:"state"`
	Progress      int    `json:"progress"`
	Details       string `json:"details"`
}

// Receipt is a verified sealed result with a local copy marker.
type Receipt struct {
	CompartmentID string         `json:"compartmentId"`
	TaskType      string         `json:"taskType"`
	Output        map[string]any `json:"output"`
	Checksum      string         `json:"checksum"`
	Verified      bool           `json:"verified"`
	RemainingTTL  int64          `json:"remainingTtlSeconds"`
	ArchivedAt    string         `json:"archivedAt"`
}

// Audit is one durable history row.
type Audit struct {
	ID            string `json:"id"`
	CompartmentID string `json:"compartmentId"`
	TaskType      string `json:"taskType"`
	FinalState    string `json:"finalState"`
	Checksum      string `json:"checksum"`
	DurationMs    int64  `json:"durationMs"`
	CompletedAt   string `json:"completedAt"`
}

// Worker is one fleet liveness entry.
type Worker struct {
	WorkerID   string `json:"workerId"`
	Status     string `json:"status"`
	Healthy    bool   `json:"healthy"`
	ActiveJob  string `json:"activeCompartmentId"`
	LastSeenSx int64  `json:"secondsSinceLastHeartbeat"`
}

// Client talks to the control plane REST API.
type Client struct {
	cfg Config
	hc  *http.Client
}

// NewClient builds a client from config.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultConfig().BaseURL
	}
	return &Client{cfg: cfg, hc: &http.Client{Timeout: 15 * time.Second}}
}

// ConfigDir returns the app data directory, creating it on demand.
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "batchseal")
	if err := os.MkdirAll(filepath.Join(dir, "receipts"), 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// LoadConfig reads persisted settings or defaults.
func LoadConfig() Config {
	cfg := DefaultConfig()
	dir, err := ConfigDir()
	if err != nil {
		return cfg
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return cfg
	}
	var saved Config
	if err := json.Unmarshal(raw, &saved); err != nil {
		return cfg
	}
	if saved.BaseURL != "" {
		cfg.BaseURL = saved.BaseURL
	}
	cfg.APIKey = saved.APIKey
	if saved.Clearance != "" {
		cfg.Clearance = saved.Clearance
	}
	if saved.OwnerID != "" {
		cfg.OwnerID = saved.OwnerID
	}
	return cfg
}

// SaveConfig persists settings (API key included: local desktop, user-owned file).
func SaveConfig(cfg Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), raw, 0o600)
}

func (c *Client) do(method, path string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, strings.TrimRight(c.cfg.BaseURL, "/")+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Context-Clearance", c.cfg.Clearance)
	if strings.TrimSpace(c.cfg.APIKey) != "" {
		req.Header.Set("X-API-Key", strings.TrimSpace(c.cfg.APIKey))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("control plane unreachable at %s: %w", c.cfg.BaseURL, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, firstLine(raw))
	}
	return raw, resp.StatusCode, nil
}

func firstLine(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

// Dispatch creates one compartment batch from numeric values.
func (c *Client) Dispatch(taskType string, values []float64) (string, error) {
	in := make([]any, len(values))
	for i, v := range values {
		in[i] = v
	}
	raw, _, err := c.do("POST", "/api/v1/compartments", map[string]any{
		"context":  c.cfg.Clearance,
		"ownerId":  c.cfg.OwnerID,
		"taskType": taskType,
		"payload":  map[string]any{"batchSize": len(values), "inputValues": in},
	})
	if err != nil {
		return "", err
	}
	var out struct {
		CompartmentID string `json:"compartmentId"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.CompartmentID == "" {
		return "", fmt.Errorf("dispatch returned no compartment ID")
	}
	return out.CompartmentID, nil
}

// DispatchRaw creates a batch with a caller-supplied payload (e.g. chaos flags).
func (c *Client) DispatchRaw(taskType string, payload map[string]any) (string, error) {
	raw, _, err := c.do("POST", "/api/v1/compartments", map[string]any{
		"context":  c.cfg.Clearance,
		"ownerId":  c.cfg.OwnerID,
		"taskType": taskType,
		"payload":  payload,
	})
	if err != nil {
		return "", err
	}
	var out struct {
		CompartmentID string `json:"compartmentId"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out.CompartmentID, nil
}

// Status polls one compartment.
func (c *Client) Status(id string) (Compartment, error) {
	var comp Compartment
	raw, _, err := c.do("GET", "/api/v1/compartments/"+id, nil)
	if err != nil {
		return comp, err
	}
	err = json.Unmarshal(raw, &comp)
	return comp, err
}

// VerifySeal recomputes SHA-256 over the canonical JSON output.
// It mirrors the worker's ComputeChecksum: encoding/json sorts map keys,
// so marshaling the decoded output reproduces the sealed bytes.
func VerifySeal(output map[string]any, checksum string) bool {
	raw, err := json.Marshal(output)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]) == strings.ToLower(strings.TrimSpace(checksum))
}

// Receipt fetches a dead drop, verifies the seal, and saves a local copy.
func (c *Client) Receipt(id string) (Receipt, error) {
	var r Receipt
	raw, code, err := c.do("GET", "/api/v1/compartments/"+id+"/deaddrop", nil)
	if err != nil {
		if code == 404 {
			return r, fmt.Errorf("not sealed yet (or expired with no audit copy)")
		}
		return r, err
	}
	var dd struct {
		CompartmentID string         `json:"compartmentId"`
		TaskType      string         `json:"taskType"`
		Output        map[string]any `json:"output"`
		Checksum      string         `json:"checksum"`
		RemainingTTL  int64          `json:"remainingTtlSeconds"`
		ArchivedAt    string         `json:"archivedAt"`
	}
	if err := json.Unmarshal(raw, &dd); err != nil {
		return r, err
	}
	r = Receipt{
		CompartmentID: dd.CompartmentID,
		TaskType:      dd.TaskType,
		Output:        dd.Output,
		Checksum:      dd.Checksum,
		RemainingTTL:  dd.RemainingTTL,
		ArchivedAt:    dd.ArchivedAt,
	}
	r.Verified = VerifySeal(dd.Output, dd.Checksum)
	if err := saveReceipt(r); err != nil {
		return r, fmt.Errorf("sealed and verified=%v but local save failed: %w", r.Verified, err)
	}
	return r, nil
}

func saveReceipt(r Receipt) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	safe := strings.Map(func(ch rune) rune {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' {
			return ch
		}
		return '_'
	}, r.CompartmentID)
	return os.WriteFile(filepath.Join(dir, "receipts", safe+".json"), raw, 0o600)
}

// ListReceipts returns locally saved sealed copies (offline-capable history).
func ListReceipts() ([]Receipt, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "receipts", "*.json"))
	if err != nil {
		return nil, err
	}
	out := make([]Receipt, 0, len(files))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var r Receipt
		if err := json.Unmarshal(raw, &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// Audits lists durable history rows, optionally filtered by owner.
func (c *Client) Audits(owner string) ([]Audit, error) {
	path := "/api/v1/audits"
	if owner != "" {
		path += "?ownerId=" + owner
	}
	raw, _, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	// Control plane serializes camelCase fields; accept snake_case too.
	var rows []struct {
		Audit
		CompartmentIDSn string `json:"compartment_id"`
		TaskTypeSn      string `json:"task_type"`
		FinalStateSn    string `json:"final_state"`
		DurationMsSn    int64  `json:"duration_ms"`
		CompletedAtSn   string `json:"completed_at"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make([]Audit, 0, len(rows))
	for _, r := range rows {
		a := r.Audit
		if a.CompartmentID == "" {
			a.CompartmentID = r.CompartmentIDSn
		}
		if a.TaskType == "" {
			a.TaskType = r.TaskTypeSn
		}
		if a.FinalState == "" {
			a.FinalState = r.FinalStateSn
		}
		if a.DurationMs == 0 {
			a.DurationMs = r.DurationMsSn
		}
		if a.CompletedAt == "" {
			a.CompletedAt = r.CompletedAtSn
		}
		out = append(out, a)
	}
	return out, nil
}

// Workers returns fleet liveness.
func (c *Client) Workers() ([]Worker, error) {
	raw, _, err := c.do("GET", "/api/v1/workers", nil)
	if err != nil {
		return nil, err
	}
	var out []Worker
	err = json.Unmarshal(raw, &out)
	return out, err
}

// Redrive requeues dead-letter entries for another attempt.
func (c *Client) Redrive(limit int) (int, error) {
	if limit <= 0 {
		limit = 10
	}
	raw, _, err := c.do("POST", fmt.Sprintf("/api/v1/workers/dlq/redrive?limit=%d", limit), nil)
	if err != nil {
		return 0, err
	}
	var out struct {
		Redriven int `json:"redriven"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, err
	}
	return out.Redriven, nil
}

// DLQSize returns the current dead-letter depth.
func (c *Client) DLQSize() (int, error) {
	raw, _, err := c.do("GET", "/api/v1/workers/dlq?limit=1", nil)
	if err != nil {
		return 0, err
	}
	var out struct {
		Size int `json:"size"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, err
	}
	return out.Size, nil
}

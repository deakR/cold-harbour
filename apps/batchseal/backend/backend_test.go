package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// fakePlane emulates the control-plane endpoints BatchSeal uses.
func fakePlane(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/compartments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-Context-Clearance") == "" {
			http.Error(w, "clearance required", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"compartmentId":"cpt_test_1","status":"QUEUED"}`))
	})
	mux.HandleFunc("/api/v1/compartments/cpt_test_1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"compartmentId":"cpt_test_1","state":"RUNNING","progress":50}`))
	})
	mux.HandleFunc("/api/v1/compartments/cpt_test_1/deaddrop", func(w http.ResponseWriter, r *http.Request) {
		output := map[string]any{"processedCount": 3, "reducedSum": 60, "status": "SUCCESS"}
		raw, _ := json.Marshal(output)
		sum := sha256Hex(raw)
		resp, _ := json.Marshal(map[string]any{
			"compartmentId": "cpt_test_1", "taskType": "DATA_REDUCTION",
			"output": output, "checksum": sum, "remainingTtlSeconds": 3599,
		})
		_, _ = w.Write(resp)
	})
	mux.HandleFunc("/api/v1/audits", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"1","compartmentId":"cpt_test_1","taskType":"DATA_REDUCTION",` +
			`"finalState":"PURGED","checksum":"x","durationMs":9,"completedAt":"2026-09-03T10:00:00Z"}]`))
	})
	mux.HandleFunc("/api/v1/workers", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"workerId":"w1","status":"IDLE","healthy":true}]`))
	})
	mux.HandleFunc("/api/v1/workers/dlq", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"stream":"coldharbor:jobs:dlq","size":2,"entries":[]}`))
	})
	mux.HandleFunc("/api/v1/workers/dlq/redrive", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`{"redriven":2,"ids":["cpt_a","cpt_b"]}`))
	})
	return httptest.NewServer(mux)
}

func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	return NewClient(Config{BaseURL: srv.URL, Clearance: "INNIE", OwnerID: "usr_t"})
}

func TestDispatchStatusReceiptFlow(t *testing.T) {
	srv := fakePlane(t)
	defer srv.Close()
	c := testClient(t, srv)

	id, err := c.Dispatch("DATA_REDUCTION", []float64{10, 25, 25})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if id != "cpt_test_1" {
		t.Fatalf("unexpected id %q", id)
	}
	st, err := c.Status(id)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.State != "RUNNING" || st.Progress != 50 {
		t.Fatalf("unexpected status %+v", st)
	}
	rc, err := c.Receipt(id)
	if err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if !rc.Verified {
		t.Fatalf("seal should verify: %+v", rc)
	}
	if rc.Output["reducedSum"] != float64(60) {
		t.Fatalf("unexpected output %+v", rc.Output)
	}
	saved, err := ListReceipts()
	if err != nil || len(saved) != 1 {
		t.Fatalf("expected 1 saved receipt, got %d (%v)", len(saved), err)
	}
}

func TestVerifySealRejectsTamperedOutput(t *testing.T) {
	good := map[string]any{"a": 1}
	raw, _ := json.Marshal(good)
	if !VerifySeal(good, sha256Hex(raw)) {
		t.Fatal("valid seal rejected")
	}
	if VerifySeal(map[string]any{"a": 2}, sha256Hex(raw)) {
		t.Fatal("tampered output accepted")
	}
}

func TestAuditsWorkersDlq(t *testing.T) {
	srv := fakePlane(t)
	defer srv.Close()
	c := testClient(t, srv)

	audits, err := c.Audits("usr_t")
	if err != nil || len(audits) != 1 || audits[0].FinalState != "PURGED" {
		t.Fatalf("audits: %+v %v", audits, err)
	}
	workers, err := c.Workers()
	if err != nil || len(workers) != 1 || !workers[0].Healthy {
		t.Fatalf("workers: %+v %v", workers, err)
	}
	size, err := c.DLQSize()
	if err != nil || size != 2 {
		t.Fatalf("dlq size: %d %v", size, err)
	}
	n, err := c.Redrive(10)
	if err != nil || n != 2 {
		t.Fatalf("redrive: %d %v", n, err)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	want := Config{BaseURL: "http://x:8080", APIKey: "k", Clearance: "OUTIE", OwnerID: "u"}
	if err := SaveConfig(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := LoadConfig()
	if got != want {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

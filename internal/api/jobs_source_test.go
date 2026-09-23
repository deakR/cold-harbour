package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestPostJobSource(t *testing.T) {
	pool := phaseCDB(t)
	srv := phaseCServer(t, pool)
	defer srv.Close()
	_, key := createTenantKey(t, pool, "sentinel")

	t.Run("missing defaults to cold-harbour", func(t *testing.T) {
		status, body := postJobRaw(t, srv.URL, key, map[string]string{
			"input":   "hello",
			"jobType": "redact",
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body %s", status, body)
		}
		var resp map[string]string
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatal(err)
		}
		var source string
		err := pool.QueryRow(context.Background(), `SELECT source FROM job_accepts WHERE redis_job_id = $1`, resp["jobId"]).Scan(&source)
		if err != nil {
			t.Fatal(err)
		}
		if source != "cold-harbour" {
			t.Fatalf("source = %q", source)
		}
	})

	t.Run("sentinel", func(t *testing.T) {
		status, body := postJobRaw(t, srv.URL, key, map[string]string{
			"input":   "hello",
			"jobType": "redact",
			"source":  "sentinel",
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body %s", status, body)
		}
		var resp map[string]string
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatal(err)
		}
		var source string
		err := pool.QueryRow(context.Background(), `SELECT source FROM job_accepts WHERE redis_job_id = $1`, resp["jobId"]).Scan(&source)
		if err != nil {
			t.Fatal(err)
		}
		if source != "sentinel" {
			t.Fatalf("source = %q", source)
		}
	})

	t.Run("bad source", func(t *testing.T) {
		status, body := postJobRaw(t, srv.URL, key, map[string]string{
			"input":   "hello",
			"jobType": "redact",
			"source":  "other",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d body %s", status, body)
		}
		var errBody map[string]string
		if err := json.Unmarshal(body, &errBody); err != nil {
			t.Fatal(err)
		}
		if errBody["field"] != "source" {
			t.Fatalf("field = %q", errBody["field"])
		}
	})
}

func postJobRaw(t *testing.T, base, key string, body map[string]string) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/v1/jobs", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	authed(req, key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, b
}

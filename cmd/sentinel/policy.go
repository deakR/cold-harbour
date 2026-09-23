package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"coldharbour/internal/detect"
	"coldharbour/internal/policy"
)

type maskSettings struct {
	maxInline int
	kinds     []detect.Kind
	mode      detect.Mode
	etag      string
}

var active atomic.Value

func loadInitial(controlURL, stateDir, policyFile, key string) (policy.Doc, string, error) {
	var fileDoc policy.Doc
	hasFile := false
	if policyFile != "" {
		doc, err := readDoc(policyFile)
		if err != nil {
			return policy.Doc{}, "", err
		}
		fileDoc, hasFile = doc, true
	}
	if controlURL != "" {
		doc, etag, _, err := fetchPolicy(controlURL+"/v1/policy", key, "")
		if err == nil {
			_ = writeCache(stateDir, doc, etag)
			return doc, etag, nil
		}
		if doc, etag, err := readCache(stateDir); err == nil {
			return doc, etag, nil
		}
		if hasFile {
			return fileDoc, "", nil
		}
		return policy.Doc{}, "", errors.New("no policy")
	}
	if hasFile {
		return fileDoc, "", nil
	}
	return policy.Doc{}, "", errors.New("no policy")
}

func poll(ctx context.Context, controlURL, stateDir, key string, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st := active.Load().(maskSettings)
			doc, etag, same, err := fetchPolicy(controlURL+"/v1/policy", key, st.etag)
			if err != nil || same {
				continue
			}
			_ = writeCache(stateDir, doc, etag)
			active.Store(settingsFrom(doc, etag))
		}
	}
}

func fetchPolicy(url, key, etag string) (policy.Doc, string, bool, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return policy.Doc{}, "", false, err
	}
	req.Header.Set("X-API-Key", key)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return policy.Doc{}, "", false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified {
		return policy.Doc{}, etag, true, nil
	}
	if res.StatusCode != http.StatusOK {
		return policy.Doc{}, "", false, fmt.Errorf("policy status %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return policy.Doc{}, "", false, err
	}
	doc, err := policy.Parse(raw)
	if err != nil {
		return policy.Doc{}, "", false, err
	}
	return doc, res.Header.Get("ETag"), false, nil
}

func settingsFrom(doc policy.Doc, etag string) maskSettings {
	mode := detect.Mode(doc.Mode)
	if mode == "" {
		mode = detect.ModeRedact
	}
	max := doc.MaxInlineBytes
	if max < 1 {
		max = 65536
	}
	return maskSettings{
		maxInline: max,
		kinds:     append([]detect.Kind(nil), doc.Detectors...),
		mode:      mode,
		etag:      etag,
	}
}

type cachedPolicy struct {
	ETag string     `json:"etag"`
	Doc  policy.Doc `json:"document"`
}

func writeCache(dir string, doc policy.Doc, etag string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(cachedPolicy{ETag: etag, Doc: doc})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "policy.json"), raw, 0o600) //#nosec G306 G304 -- operator state directory
}

func readCache(dir string) (policy.Doc, string, error) {
	if dir == "" {
		return policy.Doc{}, "", errors.New("no cache")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "policy.json")) //#nosec G304 -- operator state directory
	if err != nil {
		return policy.Doc{}, "", err
	}
	var cached cachedPolicy
	if err := json.Unmarshal(raw, &cached); err != nil {
		return policy.Doc{}, "", err
	}
	return cached.Doc, cached.ETag, nil
}

func readDoc(path string) (policy.Doc, error) {
	raw, err := os.ReadFile(path) //#nosec G304 -- operator policy file
	if err != nil {
		return policy.Doc{}, err
	}
	return policy.Parse(raw)
}

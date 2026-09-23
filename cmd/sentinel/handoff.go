package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"coldharbour/internal/detect"
)

const (
	handoffQueueCap     = 1000
	handoffBreakerLimit = 5
	handoffBackoffStart = 10 * time.Millisecond
	handoffBackoffCap   = 200 * time.Millisecond
)

type handoff struct {
	baseURL       string
	apiKey        string
	allowFailOpen bool
	client        *http.Client
	q             chan handoffItem
	failures      atomic.Int32
	open          atomic.Bool
	allowProbe    atomic.Bool
	failOpenCount atomic.Int64
	wg            sync.WaitGroup
}

type handoffItem struct {
	line  string
	reply chan handoffResult
}

type handoffResult struct {
	jobID string
	ok    bool
}

func newHandoff(baseURL, apiKey string, allowFailOpen bool) *handoff {
	h := &handoff{
		baseURL:       baseURL,
		apiKey:        apiKey,
		allowFailOpen: allowFailOpen,
		client:        &http.Client{Timeout: 250 * time.Millisecond},
		q:             make(chan handoffItem, handoffQueueCap),
	}
	if baseURL != "" {
		h.wg.Add(1)
		go h.loop()
	}
	return h
}

func (h *handoff) close() {
	if h == nil || h.baseURL == "" {
		return
	}
	close(h.q)
	h.wg.Wait()
}

func (h *handoff) handle(line string, failPolicy string, kinds []detect.Kind, mode detect.Mode) maskResult {
	if h == nil || h.baseURL == "" {
		return applyFailPolicy(line, failPolicy, kinds, mode, h)
	}
	if h.open.Load() && !h.allowProbe.CompareAndSwap(true, false) {
		return applyFailPolicy(line, failPolicy, kinds, mode, h)
	}
	item := handoffItem{line: line, reply: make(chan handoffResult, 1)}
	select {
	case h.q <- item:
		out := <-item.reply
		if out.ok {
			heldTotal.Inc()
			return maskResult{line: "[HELD job=" + out.jobID + "]", held: true}
		}
		return applyFailPolicy(line, failPolicy, kinds, mode, h)
	default:
		if h.open.Load() {
			h.allowProbe.Store(true)
		}
		return applyFailPolicy(line, failPolicy, kinds, mode, h)
	}
}

func applyFailPolicy(line, failPolicy string, kinds []detect.Kind, mode detect.Mode, h *handoff) maskResult {
	switch failPolicy {
	case "inline":
		if mode == "" {
			mode = detect.ModeRedact
		}
		if len(kinds) == 0 {
			kinds = maskKinds
		}
		out, counts := detect.Apply(line, detect.Scan(line, kinds), mode)
		return maskResult{line: out, counts: counts}
	case "open":
		if h != nil && h.allowFailOpen {
			h.failOpenCount.Add(1)
			return maskResult{line: line}
		}
	}
	return maskResult{line: "[DROPPED reason=handoff_unavailable]", dropped: true}
}

func (h *handoff) loop() {
	defer h.wg.Done()
	for item := range h.q {
		if h.open.Load() {
			jobID, err := h.post(item.line)
			if err == nil {
				h.failures.Store(0)
				h.open.Store(false)
				h.allowProbe.Store(false)
				item.reply <- handoffResult{jobID: jobID, ok: true}
				continue
			}
			handoffErrors.Inc()
			item.reply <- handoffResult{}
			time.AfterFunc(time.Second, func() { h.allowProbe.Store(true) })
			continue
		}
		backoff := handoffBackoffStart
		for {
			jobID, err := h.post(item.line)
			if err == nil {
				h.failures.Store(0)
				h.open.Store(false)
				item.reply <- handoffResult{jobID: jobID, ok: true}
				break
			}
			handoffErrors.Inc()
			n := h.failures.Add(1)
			if n >= handoffBreakerLimit {
				h.open.Store(true)
				h.allowProbe.Store(true)
				item.reply <- handoffResult{}
				break
			}
			time.Sleep(backoff)
			backoff *= 2
			if backoff > handoffBackoffCap {
				backoff = handoffBackoffCap
			}
		}
	}
}

func (h *handoff) post(line string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"input":   line,
		"jobType": "redact",
		"source":  "sentinel",
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, h.baseURL+"/v1/jobs", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", h.apiKey)
	res, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", errHandoffStatus
	}
	var resp struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if resp.JobID == "" {
		return "", errHandoffStatus
	}
	return resp.JobID, nil
}

var errHandoffStatus = errors.New("handoff status")

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"coldharbour/internal/detect"
)

type windowCounters struct {
	mu      sync.Mutex
	from    time.Time
	counts  map[detect.Kind]int64
	held    int64
	dropped int64
}

func newWindowCounters(at time.Time) *windowCounters {
	return &windowCounters{
		from:   at,
		counts: map[detect.Kind]int64{},
	}
}

func (w *windowCounters) add(result maskResult) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for kind, n := range result.counts {
		w.counts[kind] += int64(n)
	}
	if result.held {
		w.held++
	}
	if result.dropped {
		w.dropped++
	}
}

type eventBatch struct {
	NodeID        string         `json:"nodeId"`
	From          time.Time      `json:"from"`
	To            time.Time      `json:"to"`
	PolicyVersion int            `json:"policyVersion"`
	Counts        map[string]int `json:"counts"`
	Held          int64          `json:"held"`
	Dropped       int64          `json:"dropped"`
	FailOpen      int64          `json:"failOpen"`
}

func (w *windowCounters) take(to time.Time, failOpen int64, nodeID string, policyVersion int) eventBatch {
	w.mu.Lock()
	defer w.mu.Unlock()
	counts := map[string]int{}
	for kind, n := range w.counts {
		if n > 0 {
			counts[string(kind)] = int(n)
		}
	}
	batch := eventBatch{
		NodeID:        nodeID,
		From:          w.from,
		To:            to,
		PolicyVersion: policyVersion,
		Counts:        counts,
		Held:          w.held,
		Dropped:       w.dropped,
		FailOpen:      failOpen,
	}
	w.from = to
	w.counts = map[detect.Kind]int64{}
	w.held = 0
	w.dropped = 0
	return batch
}

func (w *windowCounters) restore(batch eventBatch) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.from.After(batch.From) {
		w.from = batch.From
	}
	for kind, n := range batch.Counts {
		w.counts[detect.Kind(kind)] += int64(n)
	}
	w.held += batch.Held
	w.dropped += batch.Dropped
}

type eventsLoop struct {
	baseURL  string
	apiKey   string
	nodeID   string
	interval time.Duration
	client   *http.Client
	window   *windowCounters
	handoff  *handoff
	stop     chan struct{}
	wg       sync.WaitGroup
}

func newEventsLoop(baseURL, apiKey, nodeID string, interval time.Duration, h *handoff) *eventsLoop {
	if baseURL == "" {
		return nil
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	e := &eventsLoop{
		baseURL:  baseURL,
		apiKey:   apiKey,
		nodeID:   nodeID,
		interval: interval,
		client:   &http.Client{Timeout: 2 * time.Second},
		window:   newWindowCounters(time.Now().UTC()),
		handoff:  h,
		stop:     make(chan struct{}),
	}
	e.wg.Add(1)
	go e.run()
	return e
}

func (e *eventsLoop) record(result maskResult) {
	if e == nil {
		return
	}
	e.window.add(result)
}

func (e *eventsLoop) close() {
	if e == nil {
		return
	}
	close(e.stop)
	e.wg.Wait()
	e.flush()
}

func (e *eventsLoop) run() {
	defer e.wg.Done()
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
			e.flush()
		}
	}
}

func (e *eventsLoop) flush() {
	failOpen := int64(0)
	if e.handoff != nil {
		failOpen = e.handoff.failOpenCount.Swap(0)
	}
	st := active.Load().(maskSettings)
	batch := e.window.take(time.Now().UTC(), failOpen, e.nodeID, st.version)
	body, err := json.Marshal(batch)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, e.baseURL+"/v1/sentinel/events", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", e.apiKey)
	res, err := e.client.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		e.window.restore(batch)
		if e.handoff != nil && failOpen != 0 {
			e.handoff.failOpenCount.Add(failOpen)
		}
		if res != nil {
			_ = res.Body.Close()
		}
		return
	}
	_ = res.Body.Close()
}

func (b eventBatch) MarshalJSON() ([]byte, error) {
	type wire struct {
		NodeID        string         `json:"nodeId"`
		From          string         `json:"from"`
		To            string         `json:"to"`
		PolicyVersion int            `json:"policyVersion"`
		Counts        map[string]int `json:"counts"`
		Held          int64          `json:"held"`
		Dropped       int64          `json:"dropped"`
		FailOpen      int64          `json:"failOpen"`
	}
	counts := b.Counts
	if counts == nil {
		counts = map[string]int{}
	}
	return json.Marshal(wire{
		NodeID:        b.NodeID,
		From:          b.From.UTC().Format(time.RFC3339Nano),
		To:            b.To.UTC().Format(time.RFC3339Nano),
		PolicyVersion: b.PolicyVersion,
		Counts:        counts,
		Held:          b.Held,
		Dropped:       b.Dropped,
		FailOpen:      b.FailOpen,
	})
}

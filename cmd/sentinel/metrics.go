package main

import (
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	linesTotal    = prometheus.NewCounter(prometheus.CounterOpts{Name: "sentinel_lines_total"})
	detections    = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "sentinel_detections_total"}, []string{"kind"})
	lineSeconds   = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "sentinel_line_seconds"})
	heldTotal     = prometheus.NewCounter(prometheus.CounterOpts{Name: "sentinel_held_total"})
	droppedTotal  = prometheus.NewCounter(prometheus.CounterOpts{Name: "sentinel_dropped_total"})
	handoffErrors = prometheus.NewCounter(prometheus.CounterOpts{Name: "sentinel_handoff_errors_total"})
)

func serveMetrics(addr string) error {
	reg := prometheus.NewRegistry()
	reg.MustRegister(linesTotal, detections, lineSeconds, heldTotal, droppedTotal, handoffErrors)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return nil
}

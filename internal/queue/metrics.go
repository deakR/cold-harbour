package queue

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	jobsCompleted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "coldharbour_jobs_completed_total",
	})
	jobsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "coldharbour_jobs_failed_total",
	})
	jobsRetried = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "coldharbour_jobs_retried_total",
	})
	jobsBuried = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "coldharbour_jobs_buried_total",
	})
	jobDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "coldharbour_job_duration_seconds",
	})
)

func init() {
	prometheus.MustRegister(jobsCompleted, jobsFailed, jobsRetried, jobsBuried, jobDuration)
}

func ServeMetrics(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}

func observeJob(start time.Time) {
	jobDuration.Observe(time.Since(start).Seconds())
}

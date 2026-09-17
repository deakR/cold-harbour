package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"coldharbor/worker-engine/internal/config"
	"coldharbor/worker-engine/internal/engine"
	"github.com/redis/go-redis/v9"
)

func main() {
	log.Println("=====================================================")
	log.Println(" ColdHarbor Distributed Worker Engine starting up...")
	log.Println("=====================================================")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Fatal: failed to load configuration: %v", err)
	}

	log.Printf("[Config] Worker ID:         %s", cfg.WorkerID)
	log.Printf("[Config] Redis Addr:        %s (DB: %d)", cfg.RedisAddr, cfg.RedisDB)
	log.Printf("[Config] Consumer Group:    %s", cfg.ConsumerGroup)
	log.Printf("[Config] Stream Name:       %s", cfg.StreamName)
	log.Printf("[Config] DLQ Stream Name:   %s", cfg.DLQStreamName)
	log.Printf("[Config] Event Channel:     %s", cfg.EventChannel)
	log.Printf("[Config] Heartbeat:         %v (TTL: %v)", cfg.HeartbeatInterval, cfg.HeartbeatTTL)
	log.Printf("[Config] Archive TTL:       %v", cfg.ArchiveTTL)
	log.Printf("[Config] Max Retries:       %d", cfg.MaxRetries)

	// Initialize Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Verify Redis connectivity
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()

	if err := rdb.Ping(pingCtx).Err(); err != nil {
		log.Printf("Warning: Redis ping failed (%v). Worker will retry connecting in consumer loop.", err)
	} else {
		log.Println("[Redis] Connected successfully.")
	}

	// Create and initialize consumer
	consumer := engine.NewConsumer(cfg, rdb)
	if err := consumer.InitConsumerGroup(ctx); err != nil {
		log.Printf("Notice: Consumer group initialization note: %v", err)
	}

	// Start consumer engine
	consumer.Start(ctx)
	log.Printf("[Worker %s] Engine running. Listening for stream jobs...", cfg.WorkerID)

	// Metrics + health endpoint (stdlib only, Prometheus text exposition)
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = "localhost:9091"
	}
	mux := http.NewServeMux()
	metricsSrv := &http.Server{Addr: metricsAddr, Handler: mux}
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		consumer.MetricsSnapshot().WritePrometheus(w)
	})
	redisHealth := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		checkCtx, checkCancel := context.WithTimeout(r.Context(), time.Second)
		defer checkCancel()
		if err := rdb.Ping(checkCtx).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"DOWN","workerId":%q,"redis":"UNAVAILABLE"}`, cfg.WorkerID)
			return
		}
		fmt.Fprintf(w, `{"status":"UP","workerId":%q}`, cfg.WorkerID)
	}
	mux.HandleFunc("/healthz", redisHealth)
	mux.HandleFunc("/readyz", redisHealth)
	go func() {
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Metrics] server error: %v", err)
		}
	}()
	log.Printf("[Metrics] exposition on http://%s/metrics", metricsAddr)

	// Trap termination signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("[Worker %s] Received termination signal (%v). Initiating graceful shutdown...", cfg.WorkerID, sig)

	// Trigger shutdown
	cancel()
	consumer.Stop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = metricsSrv.Shutdown(shutdownCtx)

	// Close Redis connection
	if err := rdb.Close(); err != nil {
		log.Printf("Warning: error closing Redis client: %v", err)
	}

	log.Printf("[Worker %s] Shutdown completed cleanly.", cfg.WorkerID)
	fmt.Println("Worker terminated.")
}

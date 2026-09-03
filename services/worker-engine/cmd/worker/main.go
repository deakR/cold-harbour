package main

import (
	"context"
	"fmt"
	"log"
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

	// Trap termination signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("[Worker %s] Received termination signal (%v). Initiating graceful shutdown...", cfg.WorkerID, sig)

	// Trigger shutdown
	cancel()
	consumer.Stop()

	// Close Redis connection
	if err := rdb.Close(); err != nil {
		log.Printf("Warning: error closing Redis client: %v", err)
	}

	log.Printf("[Worker %s] Shutdown completed cleanly.", cfg.WorkerID)
	fmt.Println("Worker terminated.")
}

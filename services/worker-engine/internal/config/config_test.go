package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.RedisAddr != "localhost:6379" {
		t.Errorf("expected localhost:6379, got %s", cfg.RedisAddr)
	}
	if cfg.ConsumerGroup != "worker-group" {
		t.Errorf("expected worker-group, got %s", cfg.ConsumerGroup)
	}
	if cfg.StreamName != "coldharbor:jobs" {
		t.Errorf("expected coldharbor:jobs, got %s", cfg.StreamName)
	}
	if cfg.DLQStreamName != "coldharbor:jobs:dlq" {
		t.Errorf("expected coldharbor:jobs:dlq, got %s", cfg.DLQStreamName)
	}
	if cfg.EventChannel != "coldharbor:events" {
		t.Errorf("expected coldharbor:events, got %s", cfg.EventChannel)
	}
	if cfg.HeartbeatInterval != 10*time.Second {
		t.Errorf("expected 10s, got %v", cfg.HeartbeatInterval)
	}
	if cfg.HeartbeatTTL != 30*time.Second {
		t.Errorf("expected 30s, got %v", cfg.HeartbeatTTL)
	}
	if cfg.ArchiveTTL != 3600*time.Second {
		t.Errorf("expected 3600s, got %v", cfg.ArchiveTTL)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected 3, got %d", cfg.MaxRetries)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	os.Setenv("REDIS_ADDR", "redis://10.0.0.1:6380")
	os.Setenv("WORKER_ID", "test-worker-99")
	os.Setenv("MAX_RETRIES", "5")
	os.Setenv("HEARTBEAT_INTERVAL_SECONDS", "5")
	defer func() {
		os.Unsetenv("REDIS_ADDR")
		os.Unsetenv("WORKER_ID")
		os.Unsetenv("MAX_RETRIES")
		os.Unsetenv("HEARTBEAT_INTERVAL_SECONDS")
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RedisAddr != "10.0.0.1:6380" {
		t.Errorf("expected 10.0.0.1:6380, got %s", cfg.RedisAddr)
	}
	if cfg.WorkerID != "test-worker-99" {
		t.Errorf("expected test-worker-99, got %s", cfg.WorkerID)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("expected 5, got %d", cfg.MaxRetries)
	}
	if cfg.HeartbeatInterval != 5*time.Second {
		t.Errorf("expected 5s, got %v", cfg.HeartbeatInterval)
	}
}

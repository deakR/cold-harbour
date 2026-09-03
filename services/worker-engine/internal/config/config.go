package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config holds all runtime settings for the Go worker engine
type Config struct {
	RedisAddr            string
	RedisPassword        string
	RedisDB              int
	WorkerID             string
	ConsumerGroup        string
	StreamName           string
	DLQStreamName        string
	EventChannel         string
	HeartbeatInterval    time.Duration
	HeartbeatTTL         time.Duration
	ArchiveTTL           time.Duration
	MaxRetries           int
	Concurrency          int
	BlockDuration        time.Duration
	RecoveryInterval     time.Duration
	MinIdleRecoveryTime  time.Duration
}

// DefaultConfig returns production default settings
func DefaultConfig() *Config {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = uuid.New().String()[:8]
	}

	return &Config{
		RedisAddr:           "localhost:6379",
		RedisPassword:       "",
		RedisDB:             0,
		WorkerID:            fmt.Sprintf("worker-go-%s", hostname),
		ConsumerGroup:       "worker-group",
		StreamName:          "coldharbor:jobs",
		DLQStreamName:       "coldharbor:jobs:dlq",
		EventChannel:        "coldharbor:events",
		HeartbeatInterval:   10 * time.Second,
		HeartbeatTTL:        30 * time.Second,
		ArchiveTTL:          3600 * time.Second,
		MaxRetries:          3,
		Concurrency:         1,
		BlockDuration:       2 * time.Second,
		RecoveryInterval:    15 * time.Second,
		MinIdleRecoveryTime: 30 * time.Second,
	}
}

// LoadConfig loads configuration from environment variables with fallback to defaults
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	if addr := getEnv("REDIS_ADDR", getEnv("REDIS_URL", "")); addr != "" {
		cfg.RedisAddr = cleanRedisAddr(addr)
	}
	if pw := os.Getenv("REDIS_PASSWORD"); pw != "" {
		cfg.RedisPassword = pw
	}
	if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
		if db, err := strconv.Atoi(dbStr); err == nil {
			cfg.RedisDB = db
		}
	}
	if wid := os.Getenv("WORKER_ID"); wid != "" {
		cfg.WorkerID = wid
	}
	if cg := os.Getenv("CONSUMER_GROUP"); cg != "" {
		cfg.ConsumerGroup = cg
	}
	if sn := os.Getenv("STREAM_NAME"); sn != "" {
		cfg.StreamName = sn
	}
	if dlq := os.Getenv("DLQ_STREAM_NAME"); dlq != "" {
		cfg.DLQStreamName = dlq
	}
	if ec := os.Getenv("EVENT_CHANNEL"); ec != "" {
		cfg.EventChannel = ec
	}

	if hbSec := os.Getenv("HEARTBEAT_INTERVAL_SECONDS"); hbSec != "" {
		if sec, err := strconv.Atoi(hbSec); err == nil && sec > 0 {
			cfg.HeartbeatInterval = time.Duration(sec) * time.Second
		}
	}
	if hbTTL := os.Getenv("HEARTBEAT_TTL_SECONDS"); hbTTL != "" {
		if sec, err := strconv.Atoi(hbTTL); err == nil && sec > 0 {
			cfg.HeartbeatTTL = time.Duration(sec) * time.Second
		}
	}
	if archTTL := os.Getenv("ARCHIVE_TTL_SECONDS"); archTTL != "" {
		if sec, err := strconv.Atoi(archTTL); err == nil && sec > 0 {
			cfg.ArchiveTTL = time.Duration(sec) * time.Second
		}
	}
	if retries := os.Getenv("MAX_RETRIES"); retries != "" {
		if r, err := strconv.Atoi(retries); err == nil && r >= 0 {
			cfg.MaxRetries = r
		}
	}
	if conc := os.Getenv("WORKER_CONCURRENCY"); conc != "" {
		if c, err := strconv.Atoi(conc); err == nil && c > 0 {
			cfg.Concurrency = c
		}
	}
	if blockMs := os.Getenv("BLOCK_DURATION_MS"); blockMs != "" {
		if ms, err := strconv.Atoi(blockMs); err == nil && ms > 0 {
			cfg.BlockDuration = time.Duration(ms) * time.Millisecond
		}
	}
	if recSec := os.Getenv("RECOVERY_INTERVAL_SECONDS"); recSec != "" {
		if sec, err := strconv.Atoi(recSec); err == nil && sec > 0 {
			cfg.RecoveryInterval = time.Duration(sec) * time.Second
		}
	}
	if idleSec := os.Getenv("MIN_IDLE_RECOVERY_SECONDS"); idleSec != "" {
		if sec, err := strconv.Atoi(idleSec); err == nil && sec > 0 {
			cfg.MinIdleRecoveryTime = time.Duration(sec) * time.Second
		}
	}

	return cfg, nil
}

func getEnv(primary, fallback string) string {
	if val := os.Getenv(primary); val != "" {
		return val
	}
	return fallback
}

func cleanRedisAddr(addr string) string {
	// If standard redis:// URL is given, strip protocol
	addr = strings.TrimPrefix(addr, "redis://")
	addr = strings.TrimPrefix(addr, "tcp://")
	return addr
}

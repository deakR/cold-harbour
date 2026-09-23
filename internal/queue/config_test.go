package queue

import (
	"errors"
	"testing"
	"time"
)

func TestWorkerConfigValid(t *testing.T) {
	if err := (WorkerConfig{}).valid(); !errors.Is(err, errEmptyConsumer) {
		t.Fatalf("empty consumer err = %v, want errEmptyConsumer", err)
	}
	tooSmall := WorkerConfig{Consumer: "w1", Idle: 10 * time.Millisecond, Poll: 20 * time.Millisecond}
	if err := tooSmall.valid(); !errors.Is(err, errIdleTooSmall) {
		t.Fatalf("idle < 2*poll err = %v, want errIdleTooSmall", err)
	}
	ok := WorkerConfig{Consumer: "w1", Idle: 40 * time.Millisecond, Poll: 20 * time.Millisecond}
	if err := ok.valid(); err != nil {
		t.Fatalf("valid config err = %v", err)
	}
}

func TestLoadWorkerConfigFrom(t *testing.T) {
	emptyEnv := func(string) string { return "" }
	noHost := func() (string, error) { return "", nil }
	_, err := loadWorkerConfigFrom(nil, emptyEnv, noHost)
	if !errors.Is(err, errEmptyConsumer) {
		t.Fatalf("empty name err = %v, want errEmptyConsumer", err)
	}
	fromHost, err := loadWorkerConfigFrom(nil, emptyEnv, func() (string, error) { return "worker-a", nil })
	if err != nil {
		t.Fatal(err)
	}
	if fromHost.Consumer != "worker-a" {
		t.Fatalf("hostname consumer = %q, want worker-a", fromHost.Consumer)
	}

	fromEnv, err := loadWorkerConfigFrom(nil, func(key string) string {
		if key == "CONSUMER" {
			return "from-env"
		}
		return ""
	}, noHost)
	if err != nil {
		t.Fatal(err)
	}
	if fromEnv.Consumer != "from-env" {
		t.Fatalf("env consumer = %q, want from-env", fromEnv.Consumer)
	}
	if fromEnv.Idle != 30*time.Second || fromEnv.Poll != 10*time.Second {
		t.Fatalf("live defaults Idle=%v Poll=%v, want 30s / 10s", fromEnv.Idle, fromEnv.Poll)
	}

	fromFlag, err := loadWorkerConfigFrom([]string{"-consumer", "from-flag"}, func(string) string {
		return "from-env"
	}, noHost)
	if err != nil {
		t.Fatal(err)
	}
	if fromFlag.Consumer != "from-flag" {
		t.Fatalf("flag consumer = %q, want from-flag", fromFlag.Consumer)
	}
}

func TestTestWorkerConfigTimes(t *testing.T) {
	cfg := testWorkerConfig("w1")
	if cfg.Idle != 50*time.Millisecond || cfg.Poll != 20*time.Millisecond {
		t.Fatalf("test times Idle=%v Poll=%v, want 50ms / 20ms", cfg.Idle, cfg.Poll)
	}
	if err := cfg.valid(); err != nil {
		t.Fatal(err)
	}
}

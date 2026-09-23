package queue

import (
	"errors"
	"flag"
	"io"
	"os"
	"time"
)

var (
	errEmptyConsumer = errors.New("consumer name is required")
	errIdleTooSmall  = errors.New("idle must be at least 2x poll")
)

type WorkerConfig struct {
	Consumer string
	Idle     time.Duration
	Poll     time.Duration
}

func (c WorkerConfig) valid() error {
	if c.Consumer == "" {
		return errEmptyConsumer
	}
	if c.Idle < 2*c.Poll {
		return errIdleTooSmall
	}
	return nil
}

func testWorkerConfig(consumer string) WorkerConfig {
	return WorkerConfig{
		Consumer: consumer,
		Idle:     50 * time.Millisecond,
		Poll:     20 * time.Millisecond,
	}
}

func LoadWorkerConfig() (WorkerConfig, error) {
	return loadWorkerConfigFrom(os.Args[1:], os.Getenv, os.Hostname)
}

func loadWorkerConfigFrom(args []string, getenv func(string) string, hostname func() (string, error)) (WorkerConfig, error) {
	fs := flag.NewFlagSet("coldharbour", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	name := fs.String("consumer", "", "consumer name")
	if err := fs.Parse(args); err != nil {
		return WorkerConfig{}, err
	}
	consumer := *name
	if consumer == "" {
		consumer = getenv("CONSUMER")
	}
	if consumer == "" {
		host, err := hostname()
		if err != nil {
			return WorkerConfig{}, err
		}
		consumer = host
	}
	cfg := WorkerConfig{
		Consumer: consumer,
		Idle:     30 * time.Second,
		Poll:     10 * time.Second,
	}
	return cfg, cfg.valid()
}

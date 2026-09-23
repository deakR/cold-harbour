package main

import (
	"context"
	"fmt"
	"log"

	"coldharbour/internal/journal"
	"coldharbour/internal/mask"
	"coldharbour/internal/queue"
	"coldharbour/internal/redact"
	"coldharbour/internal/runner"
	"coldharbour/internal/seal"
)

func main() {
	cfg, err := queue.LoadWorkerConfig()
	if err != nil {
		log.Fatalf("worker: load config: %v", err)
	}
	dsn := journal.PostgresDSN()
	store, err := journal.OpenJournal(dsn)
	if err != nil {
		log.Fatalf("worker: open journal: %v", err)
	}
	defer store.Close()
	keys, err := seal.OpenPostgresKeyStore(dsn)
	if err != nil {
		log.Fatalf("worker: open key store: %v", err)
	}
	defer keys.Close()
	receipts, err := seal.OpenPostgresReceiptStore(dsn)
	if err != nil {
		log.Fatalf("worker: open receipt store: %v", err)
	}
	defer receipts.Close()
	priv, err := seal.LoadSigningKey()
	if err != nil {
		log.Fatalf("worker: load signing key: %v", err)
	}
	reg := runner.NewRegistry(redact.Runner{}, mask.Runner{})
	if err := reg.RequireListedTypes(); err != nil {
		log.Fatalf("worker: job types: %v", err)
	}
	stream := queue.OpenJobs(queue.RedisAddr())
	if err := queue.PrepareGroup(context.Background(), stream); err != nil {
		log.Fatalf("worker: prepare group: %v", err)
	}
	go func() {
		if err := queue.ServeMetrics(":9100"); err != nil {
			log.Printf("worker: metrics: %v", err)
		}
	}()
	if err := queue.RunGroup(context.Background(), stream, cfg, queue.Deps{
		Registry:   reg,
		Journal:    store,
		Keys:       keys,
		Receipts:   receipts,
		SigningKey: priv,
		Emit: func(result journal.JobResult) {
			fmt.Println(journal.FormatJobLine(result))
			fmt.Println(result.History)
		},
	}); err != nil {
		log.Fatalf("worker: run: %v", err)
	}
}

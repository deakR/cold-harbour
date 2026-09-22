package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"coldharbour/internal/checkpoint"
	"coldharbour/internal/journal"
	"coldharbour/internal/mask"
	"coldharbour/internal/queue"
	"coldharbour/internal/redact"
	"coldharbour/internal/runner"
	"coldharbour/internal/seal"
)

func main() {
	printM4CheckpointDemo()

	cfg, err := queue.LoadWorkerConfig()
	if err != nil {
		panic(err)
	}
	dsn := journal.PostgresDSN()
	store, err := journal.OpenJournal(dsn)
	if err != nil {
		panic(err)
	}
	defer store.Close()
	keys, err := seal.OpenPostgresKeyStore(dsn)
	if err != nil {
		panic(err)
	}
	defer keys.Close()
	receipts, err := seal.OpenPostgresReceiptStore(dsn)
	if err != nil {
		panic(err)
	}
	defer receipts.Close()
	priv, err := seal.LoadSigningKey()
	if err != nil {
		panic(err)
	}
	reg := runner.NewRegistry(redact.Runner{}, mask.Runner{})
	stream := queue.OpenJobs(queue.RedisAddr())
	if err := queue.PrepareGroup(context.Background(), stream, queue.DemoJobs); err != nil {
		panic(err)
	}
	if err := queue.RunGroup(context.Background(), stream, cfg, reg, store, keys, receipts, priv, func(result journal.JobResult) {
		fmt.Println(journal.FormatJobLine(result))
		fmt.Println(result.History)
	}); err != nil {
		if errors.Is(err, checkpoint.ErrSimulatedCrash) {
			os.Exit(1)
		}
		panic(err)
	}
}

func printM4CheckpointDemo() {
	store := checkpoint.NewCheckpointStore()
	_, err := store.Run("job-5", redact.M1Fixture, checkpoint.CrashAfterStep1)
	if !errors.Is(err, checkpoint.ErrSimulatedCrash) {
		panic(err)
	}
	fmt.Println(err)
	resumed, err := store.Resume("job-5")
	if err != nil {
		panic(err)
	}
	fmt.Println(journal.FormatJobLine(journal.JobResult{ID: "job-5", Result: redact.MapResult(resumed)}))
	fmt.Println(store.Step1Passes("job-5"))
}

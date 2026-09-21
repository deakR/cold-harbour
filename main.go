package main

import (
	"context"
	"errors"
	"fmt"
	"os"
)

func main() {
	printM4CheckpointDemo()

	cfg, err := loadWorkerConfig()
	if err != nil {
		panic(err)
	}
	journal, err := OpenJournal(postgresDSN())
	if err != nil {
		panic(err)
	}
	defer journal.Close()
	stream := openJobs(redisAddr())
	if err := prepareGroup(context.Background(), stream, demoJobs); err != nil {
		panic(err)
	}
	if err := runGroup(context.Background(), stream, cfg, journal, func(result JobResult) {
		fmt.Println(formatJobLine(result))
		fmt.Println(result.History)
	}); err != nil {
		if errors.Is(err, ErrSimulatedCrash) {
			os.Exit(1)
		}
		panic(err)
	}
}

func printM4CheckpointDemo() {
	store := NewCheckpointStore()
	crashJob := Job{ID: "job-5", Input: m1Fixture}
	_, err := store.Run(crashJob, CrashAfterStep1)
	if !errors.Is(err, ErrSimulatedCrash) {
		panic(err)
	}
	fmt.Println(err)
	resumed, err := store.Resume(crashJob.ID)
	if err != nil {
		panic(err)
	}
	fmt.Println(formatJobLine(JobResult{ID: crashJob.ID, Result: resumed}))
	fmt.Println(store.Step1Passes(crashJob.ID))
}

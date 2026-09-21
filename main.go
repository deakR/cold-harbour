package main

import (
	"context"
	"errors"
	"fmt"
)

func main() {
	printM4CheckpointDemo()

	stream := openJobs(redisAddr())
	if err := seedIfEmpty(context.Background(), stream, demoJobs); err != nil {
		panic(err)
	}
	if err := runStream(context.Background(), stream, func(result JobResult) {
		fmt.Println(formatJobLine(result))
		fmt.Println(result.History)
	}); err != nil {
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

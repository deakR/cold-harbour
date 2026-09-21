package main

import (
	"errors"
	"fmt"
)

func main() {
	jobs := make(chan Job, len(demoJobs))
	results := make(chan JobResult)
	go runWorker(jobs, results)
	for _, job := range demoJobs {
		jobs <- job
	}
	close(jobs)
	for result := range results {
		fmt.Println(formatJobLine(result))
		fmt.Println(result.History)
	}

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

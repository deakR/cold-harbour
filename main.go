package main

import "fmt"

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
	}
}

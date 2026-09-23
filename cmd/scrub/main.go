package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"coldharbour/internal/queue"
)

func main() {
	stream := queue.OpenJobs(queue.RedisAddr())
	report, err := stream.ScrubPlaintext(context.Background())
	if err != nil {
		log.Fatalf("scrub: %v", err)
	}
	fmt.Printf("main deleted %d\n", len(report.MainDeleted))
	for _, id := range report.MainDeleted {
		fmt.Printf("  deleted %s\n", id)
	}
	fmt.Printf("main kept %d\n", len(report.MainKept))
	for _, kept := range report.MainKept {
		fmt.Printf("  kept %s (%s)\n", kept.ID, kept.Reason)
	}
	fmt.Printf("dlq rewritten %d\n", len(report.DLQRewritten))
	for _, id := range report.DLQRewritten {
		fmt.Printf("  rewritten %s\n", id)
	}
	fmt.Printf("dlq removed %d\n", len(report.DLQRemoved))
	for _, id := range report.DLQRemoved {
		fmt.Printf("  removed %s\n", id)
	}
	if len(report.MainKept) > 0 {
		fmt.Fprintln(os.Stderr, "kept entries still have input because a worker may still read them")
	}
}

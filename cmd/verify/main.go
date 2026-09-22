package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"time"

	"coldharbour/internal/seal"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: verify receipt --job-id ... --purged-at ... --sig ... --pub ...")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "receipt":
		os.Exit(runReceipt(os.Args[2:]))
	case "output":
		os.Exit(runOutput(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

func runReceipt(args []string) int {
	fs := flag.NewFlagSet("receipt", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jobID := fs.String("job-id", "", "redis job id")
	purgedAt := fs.String("purged-at", "", "UTC RFC3339Nano timestamp")
	sigB64 := fs.String("sig", "", "base64 Ed25519 signature")
	pubB64 := fs.String("pub", "", "base64 Ed25519 public key")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *jobID == "" || *purgedAt == "" || *sigB64 == "" || *pubB64 == "" {
		fmt.Fprintln(os.Stderr, "receipt requires --job-id --purged-at --sig --pub")
		return 2
	}
	at, err := time.Parse(time.RFC3339Nano, *purgedAt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid --purged-at")
		return 1
	}
	msg, err := seal.PurgeMessage(*jobID, at)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sig, err := base64.StdEncoding.DecodeString(*sigB64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid --sig")
		return 1
	}
	pub, err := seal.ParsePublicKey(*pubB64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid --pub")
		return 1
	}
	if !seal.Verify(pub, msg, sig) {
		return 1
	}
	return 0
}

func runOutput(args []string) int {
	fs := flag.NewFlagSet("output", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	bodyPath := fs.String("body", "", "canonical output JSON file")
	sigB64 := fs.String("sig", "", "base64 Ed25519 signature")
	pubB64 := fs.String("pub", "", "base64 Ed25519 public key")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *bodyPath == "" || *sigB64 == "" || *pubB64 == "" {
		fmt.Fprintln(os.Stderr, "output requires --body --sig --pub")
		return 2
	}
	body, err := os.ReadFile(*bodyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sig, err := base64.StdEncoding.DecodeString(*sigB64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid --sig")
		return 1
	}
	pub, err := seal.ParsePublicKey(*pubB64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid --pub")
		return 1
	}
	if !seal.Verify(pub, body, sig) {
		return 1
	}
	return 0
}

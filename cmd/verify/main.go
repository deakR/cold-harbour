package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"coldharbour/internal/ledger"
	"coldharbour/internal/seal"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: verify <receipt|output|ledger|proof> [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "receipt":
		os.Exit(runReceipt(os.Args[2:]))
	case "output":
		os.Exit(runOutput(os.Args[2:]))
	case "ledger":
		os.Exit(runLedger(os.Args[2:]))
	case "proof":
		os.Exit(runProof(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

func runLedger(args []string) int {
	fs := flag.NewFlagSet("ledger", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenant := fs.String("tenant", "", "tenant id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	id, err := uuid.Parse(*tenant)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ledger requires --tenant")
		return 2
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "POSTGRES_DSN is required")
		return 2
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer conn.Close(ctx)
	if err := ledger.Verify(ctx, conn, id); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
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
	return verifySig(*pubB64, *sigB64, msg)
}

func runOutput(args []string) int {
	fs := flag.NewFlagSet("output", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	bodyPath := fs.String("body", "", "canonical output JSON file")
	jobID := fs.String("job-id", "", "redis job id")
	tenantID := fs.String("tenant-id", "", "tenant id")
	sigB64 := fs.String("sig", "", "base64 Ed25519 signature")
	pubB64 := fs.String("pub", "", "base64 Ed25519 public key")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *bodyPath == "" || *jobID == "" || *tenantID == "" || *sigB64 == "" || *pubB64 == "" {
		fmt.Fprintln(os.Stderr, "output requires --body --job-id --tenant-id --sig --pub")
		return 2
	}
	body, err := os.ReadFile(*bodyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	msg, err := seal.OutputMessage(*jobID, *tenantID, body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return verifySig(*pubB64, *sigB64, msg)
}

func verifySig(pubB64, sigB64 string, msg []byte) int {
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid --sig")
		return 1
	}
	pub, err := seal.ParsePublicKey(pubB64)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid --pub")
		return 1
	}
	if !seal.Verify(pub, msg, sig) {
		return 1
	}
	return 0
}

func runProof(args []string) int {
	fs := flag.NewFlagSet("proof", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	leafHex := fs.String("leaf", "", "hex-encoded leaf hash")
	rootHex := fs.String("root", "", "hex-encoded Merkle root hash")
	siblingsHex := fs.String("siblings", "", "comma-separated hex-encoded sibling hashes")
	directions := fs.String("left", "", "comma-separated booleans (true/false) indicating if sibling is on left")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *leafHex == "" || *rootHex == "" {
		fmt.Fprintln(os.Stderr, "proof requires --leaf and --root")
		return 2
	}
	leaf, err := hex.DecodeString(*leafHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid --leaf hex")
		return 1
	}
	root, err := hex.DecodeString(*rootHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid --root hex")
		return 1
	}
	var siblings [][]byte
	if *siblingsHex != "" {
		for _, s := range strings.Split(*siblingsHex, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			b, err := hex.DecodeString(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid sibling hex")
				return 1
			}
			siblings = append(siblings, b)
		}
	}
	var isLeft []bool
	if *directions != "" {
		for _, d := range strings.Split(*directions, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			isLeft = append(isLeft, d == "true")
		}
	}
	if !ledger.VerifyInclusionProof(leaf, siblings, isLeft, root) {
		fmt.Fprintln(os.Stderr, "inclusion proof verification failed")
		return 1
	}
	fmt.Println("inclusion proof verified: valid")
	return 0
}

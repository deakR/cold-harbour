package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"coldharbour/internal/keys"
	"coldharbour/internal/migrate"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: coldharbour migrate | coldharbour admin create-tenant --name <name> | coldharbour admin ensure-sentinel-key --tenant-name <name> --out <path>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "migrate":
		if err := runMigrate(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "admin":
		if err := runAdmin(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}

func dsn() string {
	v := os.Getenv("POSTGRES_DSN")
	if v == "" {
		fmt.Fprintln(os.Stderr, "POSTGRES_DSN is required")
		os.Exit(2)
	}
	return v
}

func runMigrate() error {
	db, err := sql.Open("pgx", dsn())
	if err != nil {
		return err
	}
	defer db.Close()
	return migrate.Up(db)
}

func runAdmin(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: coldharbour admin create-tenant --name <name> | coldharbour admin ensure-sentinel-key --tenant-name <name> --out <path>")
	}
	switch args[0] {
	case "create-tenant":
		return runCreateTenant(args[1:])
	case "ensure-sentinel-key":
		return runEnsureSentinelKey(args[1:])
	default:
		return fmt.Errorf("unknown admin command %q", args[0])
	}
}

func runCreateTenant(args []string) error {
	fs := flag.NewFlagSet("create-tenant", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "tenant name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn())
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tenantID uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, *name).Scan(&tenantID); err != nil {
		return err
	}
	_, _, full, err := keys.Create(ctx, tx, tenantID, "admin", "admin")
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Println(full)
	return nil
}

func runEnsureSentinelKey(args []string) error {
	fs := flag.NewFlagSet("ensure-sentinel-key", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tenantName := fs.String("tenant-name", "", "tenant name")
	outPath := fs.String("out", "", "path to write the sentinel API key")
	keyName := fs.String("key-name", "compose-sentinel", "api_keys.name for a new key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tenantName == "" || *outPath == "" {
		return fmt.Errorf("usage: coldharbour admin ensure-sentinel-key --tenant-name <name> --out <path>")
	}
	if raw, err := os.ReadFile(*outPath); err == nil && strings.TrimSpace(string(raw)) != "" {
		fmt.Fprintf(os.Stderr, "sentinel key already present at %s\n", *outPath)
		return nil
	}
	ctx := context.Background()
	var last error
	for attempt := 0; attempt < 60; attempt++ {
		err := writeSentinelKey(ctx, *tenantName, *keyName, *outPath)
		if err == nil {
			fmt.Fprintf(os.Stderr, "sentinel key written to %s\n", *outPath)
			return nil
		}
		last = err
		time.Sleep(time.Second)
	}
	return fmt.Errorf("ensure-sentinel-key: %w", last)
}

func writeSentinelKey(ctx context.Context, tenantName, keyName, outPath string) error {
	conn, err := pgx.Connect(ctx, dsn())
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tenantID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM tenants WHERE name = $1`, tenantName).Scan(&tenantID)
	if err == pgx.ErrNoRows {
		err = tx.QueryRow(ctx, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, tenantName).Scan(&tenantID)
	}
	if err != nil {
		return err
	}
	_, _, full, err := keys.Create(ctx, tx, tenantID, keyName, "sentinel")
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(full+"\n"), 0o600)
}

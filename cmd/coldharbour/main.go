package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	"coldharbour/internal/keys"
	"coldharbour/internal/migrate"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: coldharbour migrate | coldharbour admin create-tenant --name <name>")
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
	if len(args) < 1 || args[0] != "create-tenant" {
		return fmt.Errorf("usage: coldharbour admin create-tenant --name <name>")
	}
	fs := flag.NewFlagSet("create-tenant", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	name := fs.String("name", "", "tenant name")
	if err := fs.Parse(args[1:]); err != nil {
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

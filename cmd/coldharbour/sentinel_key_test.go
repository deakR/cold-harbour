package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coldharbour/internal/keys"
	"coldharbour/internal/migrate"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestClassifySentinelKey(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		role string
		ok   bool
		want bool
	}{
		{name: "empty", raw: "", want: false},
		{name: "blank", raw: "  \n", want: false},
		{name: "rejected", raw: "ch_deadbeef_secret", ok: false, want: false},
		{name: "wrong role", raw: "ch_deadbeef_secret", role: "admin", ok: true, want: false},
		{name: "live", raw: "ch_deadbeef_secret\n", role: "sentinel", ok: true, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifySentinelKey(tc.raw, tc.role, tc.ok)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEnsureSentinelKeyReplacesRejectedFile(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN is empty")
	}
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(sqldb); err != nil {
		t.Fatal(err)
	}
	_ = sqldb.Close()

	path := filepath.Join(t.TempDir(), "sentinel.key")
	stale := "ch_deadbeef_not-in-database\n"
	if err := os.WriteFile(path, []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	tenant := "ensure-key-" + uuid.NewString()
	if err := runEnsureSentinelKey([]string{"--tenant-name", tenant, "--out", path}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == stale {
		t.Fatal("stale key was kept")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	p, ok := keys.Authenticate(ctx, conn, strings.TrimSpace(string(raw)))
	if !ok || p.Role != "sentinel" {
		t.Fatalf("replacement key did not authenticate as sentinel")
	}
	if err := runEnsureSentinelKey([]string{"--tenant-name", tenant, "--out", path}); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(raw) {
		t.Fatal("live key was replaced")
	}
}

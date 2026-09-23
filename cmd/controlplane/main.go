package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"coldharbour/internal/api"
	"coldharbour/internal/mask"
	"coldharbour/internal/queue"
	"coldharbour/internal/redact"
	"coldharbour/internal/runner"
	"coldharbour/internal/seal"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Fatal("controlplane: POSTGRES_DSN is required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("controlplane: postgres: %v", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		log.Fatalf("controlplane: postgres: %v", err)
	}
	rdb := queue.NewRedisClient(queue.RedisAddr())
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("controlplane: redis: %v", err)
	}
	keys, err := seal.OpenPostgresKeyStore(dsn)
	if err != nil {
		log.Fatalf("controlplane: keys: %v", err)
	}
	defer keys.Close()
	reg := runner.NewRegistry(redact.Runner{}, mask.Runner{})
	srv := api.New(db, rdb, keys, queue.OpenJobs(queue.RedisAddr()), reg.Types())
	if err := srv.StartEvents(ctx); err != nil {
		log.Fatalf("controlplane: events: %v", err)
	}
	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}
	if addr[0] != ':' {
		addr = ":" + addr
	}
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(httpServer.ListenAndServe())
}

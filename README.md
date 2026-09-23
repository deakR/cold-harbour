# Cold Harbour

Cold Harbour is an HTTP API that redacts personal data from text and proves what happened to it afterward. A tenant posts text to `POST /jobs`. A Go worker in `cmd/worker` finds an email, a US phone number, and an SSN, then replaces each match with a placeholder. When the job reaches `COMPLETED`, the worker signs the output, deletes its working copy in Redis, and writes a signed receipt for that deletion. A tenant can later download a CSV that lists every job, its checksum, and its purge time, and check that CSV against the raw signatures with no access to the database.

Two processes make up the system. `cmd/worker` is the Go process that redacts text, encrypts its working copy, signs results, and writes the audit trail. `controlplane` is a Spring Boot process that accepts the HTTP call and reads job status. `controlplane` never redacts text. It only reads and writes Redis and Postgres.

## Status

This is a work in progress, not a deployed service.

## Run it locally

You need Docker, Go 1.25, Java 21, and Maven (the `mvnw` wrapper is checked in).

1. Generate a TLS certificate for Redis and Postgres. Run this once.

   ```shell
   openssl req -x509 -newkey rsa:2048 -keyout docker/certs/server.key -out docker/certs/server.crt -days 3 -nodes -subj "/CN=localhost"
   ```

2. Set `POSTGRES_PASSWORD` and `REDIS_PASSWORD`, then start Postgres and Redis.

   ```shell
   export POSTGRES_PASSWORD=change-me
   export REDIS_PASSWORD=change-me
   docker compose up -d
   ```

   `docker/init.sql` seeds two tenants and two API keys on first start. Compose maps Postgres to port 5433 and Redis to port 6380 so they do not collide with a system install on the default ports.

3. Generate a signing key for the worker.

   ```shell
   go run ./cmd/genkey    # print an Ed25519 private and public key, base64
   export SIGNING_KEY=<the private key>
   ```

   `cmd/genkey` is not checked in. Write a one-off `main.go` that calls `ed25519.GenerateKey`, or reuse a key you already have. Keep the public key. `cmd/verify` needs it later.

4. Start the worker.

   ```shell
   export POSTGRES_DSN="postgres://coldharbour:$POSTGRES_PASSWORD@127.0.0.1:5433/coldharbour?sslmode=require"
   export REDIS_ADDR=127.0.0.1:6380
   export REDIS_TLS=1
   export REDIS_CA=docker/certs/server.crt
   go run ./cmd/worker -consumer worker-1
   ```

5. Start the control plane on a free port. Port 8080 is often already taken by another local service, so this example uses 8081.

   ```shell
   cd controlplane
   ./mvnw -q spring-boot:run "-Dspring-boot.run.arguments=--server.port=8081"
   ```

6. Post a job with a seeded tenant key.

   ```shell
   curl -X POST http://127.0.0.1:8081/jobs \
     -H "X-API-Key: ch_live_a_demo_key_aaaaaaaa" \
     -H "Content-Type: application/json" \
     -d '{"input":"Contact jane.doe@example.com or (555) 123-4567."}'
   ```

   The response has a `jobId`. Poll `GET /jobs/{jobId}` with the same key until `status` reads `COMPLETED`.

7. Optionally, run the dashboard.

   ```shell
   cd dashboard
   npm install
   npm run dev
   ```

   The dashboard submits jobs, reads live status over the `/ws/events` WebSocket, and downloads the compliance report.

## Reference

### Endpoints

All endpoints except `GET /d/{token}` require the header `X-API-Key`. A missing or revoked key returns `401`. A tenant that requests a job it did not create receives `404`, the same as for a job that does not exist.

| Method and path | What it does |
| --- | --- |
| `POST /jobs` | Queues a job. Body is `{"input": string, "jobType": string}`. `jobType` defaults to `redact` and must be listed in `internal/runner/types.txt`. The control plane encrypts `input` with a per-job AES-256-GCM key before writing Redis, and stores that key in `job_keys`. Returns `{"jobId", "status": "QUEUED"}`. |
| `GET /jobs` | Lists the calling tenant's jobs, most recent first. |
| `GET /jobs/{id}` | Returns the job's status. On `COMPLETED`, the response includes the redacted result, `signature`, and `signingKeyId`. |
| `POST /jobs/{id}/delivery-links` | Creates a one-time link. Body is `{"expiresAt": RFC3339 timestamp, "maxViews": integer}`. Returns the raw token once. |
| `GET /d/{token}` | The one route with no `X-API-Key`. Resolves the token, checks `expires_at` and `max_views`, and returns the sealed result. |
| `GET /reports/compliance?from=&to=` | Returns a CSV of the calling tenant's jobs in that date range: dispatch time, completion time, final state, checksum, signature status, purge time. |
| `GET /ws/events?apiKey=` | WebSocket. Relays each job's state transitions to the tenant that authenticated the connection. |

### Environment variables

| Variable | Read by | Meaning |
| --- | --- | --- |
| `POSTGRES_DSN` | `cmd/worker`, `controlplane` | Postgres connection string. `controlplane` falls back to `jdbc:postgresql://localhost:5433/coldharbour` with user `coldharbour` if unset. |
| `REDIS_ADDR` | `cmd/worker`, `controlplane` | Redis host and port. Defaults to `127.0.0.1:6379`. |
| `REDIS_PASSWORD` | `cmd/worker`, `controlplane` | Redis `AUTH` password. |
| `REDIS_TLS` | `cmd/worker`, `controlplane` | Set to `1` to connect over TLS. |
| `REDIS_CA` | `cmd/worker` | Path to the CA certificate that signed the Redis server certificate. |
| `SIGNING_KEY` | `cmd/worker`, `cmd/verify` | Base64 of a 64-byte Ed25519 private key. The worker signs every job output and every purge receipt with this key. |
| `CONSUMER` | `cmd/worker` | Consumer name inside the `worker-group` Redis consumer group. Two worker processes must use different names. If this and `-consumer` are both unset, the worker uses the machine hostname. |
| `VAULT_ADDR`, `VAULT_TOKEN` | `cmd/worker`, `controlplane` | If both are set, each job's AES key is wrapped through Vault Transit at `$VAULT_ADDR/v1/transit/{encrypt,decrypt}/coldharbour`. If either is unset, the raw 32-byte key is stored. Set both on the control plane and the worker, or on neither. |

### Repository layout

| Path | Contents |
| --- | --- |
| `cmd/worker` | The worker process. Reads `coldharbour:jobs`, runs the registered job type, signs and journals the result, then purges the key. Serves Prometheus metrics on `:9100/metrics`. |
| `cmd/verify` | A standalone checker. `verify output --body --job-id --tenant-id --sig --pub` checks a signed job result. `verify receipt --job-id --purged-at --sig --pub` checks a purge receipt. Neither subcommand touches Redis or Postgres. |
| `cmd/scrub` | One-time cleanup for Redis written before finished jobs dropped `input`. Deletes settled `coldharbour:jobs` entries that still have `input`. Rewrites `coldharbour:jobs:dlq` entries so `input` is gone. Pending and unread jobs are left in place. |
| `internal/redact` | The `redact` job type: the three regular expressions and the `RedactPII` function they implement. |
| `internal/mask` | The `mask` job type: replaces named fields in a JSON document. Added to prove the job-type registry needs no changes to add a job. |
| `internal/runner` | The `JobRunner` interface, the registry, and `types.txt`. That file is the only job-type list. The worker refuses to start if the registry and the file disagree. The control plane reads the same file. |
| `internal/queue` | The Redis stream, the consumer group, the dead-letter queue, and the encrypted checkpoint hash. The `input` field on a queued job is AES-GCM ciphertext. A job that reaches `COMPLETED` or `FAILED` leaves no `input` field on `coldharbour:jobs` or `coldharbour:jobs:dlq`. |
| `internal/checkpoint` | The in-memory checkpoint store used before the Redis-backed one existed. Kept for its tests. |
| `internal/seal` | AES-GCM encryption of the checkpoint, Ed25519 signing, the key store, the purge receipt store, and the optional Vault wrapping. |
| `internal/journal` | The Postgres audit row: one row per job with its checksum, its signature, and its final state. |
| `internal/events` | The Redis Pub/Sub event a job publishes on each state transition. |
| `controlplane` | The Spring Boot process: `POST /jobs`, `GET /jobs`, `GET /jobs/{id}`, delivery links, the compliance report, and the WebSocket relay. |
| `dashboard` | A React and TypeScript page that drives the API above: submit a job, watch it live, download the result, generate a link, and download the report. |
| `docker-compose.yml` | Local Postgres and Redis, both with a password and TLS. |
| `docker/init.sql` | The schema and the two seeded tenants. |

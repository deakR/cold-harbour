# Cold Harbour

Cold Harbour is an HTTP API that redacts personal data from text and proves what happened to it afterward. A tenant posts text to `POST /v1/jobs`. A Go worker in `cmd/worker` finds an email, a US phone number, and an SSN, then replaces each match with a placeholder. When the job reaches `COMPLETED`, the worker signs the output, deletes its working copy in Redis, and writes a signed receipt for that deletion. A tenant can later download a CSV that lists every job, its checksum, and its purge time, and check that CSV against the raw signatures with no access to the database.

Two processes make up the system. `cmd/worker` redacts text, encrypts its working copy, signs results, and writes the audit trail. `cmd/controlplane` accepts the HTTP call and reads job status. The control plane never redacts text. It only reads and writes Redis and Postgres.

## Status

This is a work in progress, not a deployed service.

## Run it locally

You need Docker and Go 1.25.

1. Generate a CA and a server certificate for the `postgres` and `redis` host names. Run this once.

   ```shell
   sh scripts/gen-certs.sh
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
   export POSTGRES_DSN="postgres://coldharbour:$POSTGRES_PASSWORD@127.0.0.1:5433/coldharbour?sslmode=verify-full&sslrootcert=docker/certs/ca.crt"
   export REDIS_ADDR=127.0.0.1:6380
   export REDIS_TLS=1
   export REDIS_CA=docker/certs/ca.crt
   go run ./cmd/worker -consumer worker-1
   ```

5. Start the control plane with the same `POSTGRES_DSN`, `REDIS_ADDR`, `REDIS_TLS`, and `REDIS_CA`.

   ```shell
   PORT=8081 go run ./cmd/controlplane
   ```

6. Post a job with a seeded tenant key.

   ```shell
   curl -X POST http://127.0.0.1:8081/v1/jobs \
     -H "X-API-Key: $ADMIN_KEY" \
     -H "Content-Type: application/json" \
     -d '{"input":"Contact jane.doe@example.com or (555) 123-4567."}'
   ```

   The response has a `jobId`. Poll `GET /v1/jobs/{jobId}` with the same key until `status` reads `COMPLETED`.

7. Optionally, run the dashboard.

   ```shell
   cd dashboard
   npm install
   npm run dev
   ```

   The dashboard submits jobs, reads live status over the `/v1/ws/events` WebSocket, and downloads the compliance report.

## Reference

### Endpoints

All endpoints except `GET /d/{token}` require the header `X-API-Key`. A missing or revoked key returns `401`. A tenant that requests a job it did not create receives `404`, the same as for a job that does not exist.

| Method and path | What it does |
| --- | --- |
| `POST /v1/jobs` | Queues a job. Body is `{"input": string, "jobType": string}`. `jobType` defaults to `redact` and must be a registered runner. The control plane encrypts `input` with a per-job AES-256-GCM key before writing Redis, and stores that key in `job_keys`. Returns `{"jobId", "status": "QUEUED"}`. |
| `GET /v1/jobs` | Lists the calling tenant's jobs, most recent first. |
| `GET /v1/jobs/{id}` | Returns the job's status. On `COMPLETED`, the response includes the redacted result, `signature`, and `signingKeyId`. |
| `POST /v1/jobs/{id}/delivery-links` | Creates a link after the job has an output. Body is `{"expiresAt": RFC3339 timestamp, "maxViews": integer from 1 to 100}`. `expiresAt` must be in the future and at most 30 days ahead. A bad field returns `400` with `field` set to the name. A job with no output returns `409`. Returns the raw token once. |
| `GET /d/{token}` | The one route with no `X-API-Key`. Resolves the token, checks `expires_at` and `max_views`, and returns the result. An expired link returns `410`. A link that has used its views returns `403`. |
| `GET /v1/reports/compliance?from=&to=` | Returns a CSV of the calling tenant's jobs in that date range. The header is `dispatch_time,completion_time,final_state,checksum,signature_status,purge_timestamp`. A bad `from` or `to` returns `400` with `field` set to the name. |
| `GET /v1/ws/events?apiKey=` | WebSocket. Relays each job's state transitions to the tenant that authenticated the connection. The handshake allows the request host and the hosts in `WS_ORIGIN_PATTERNS`. The default host is `localhost:5173`, which is the dashboard dev server. An `Origin` whose host is neither the request host nor one of those patterns returns 403. |

### Keys and roles

`coldharbour admin create-tenant --name paykochi` creates a tenant and prints one `admin` key. That key calls `POST /v1/keys` with `{"name","role"}`. `role` is `admin`, `app`, or `sentinel`.

`admin` manages keys and can call every `app` route. `app` submits and reads jobs, creates delivery links, downloads the compliance report, and opens the event socket. `sentinel` submits jobs. `DELETE /v1/keys/{id}` sets `revoked_at`. The next request with that key returns 401. `verify ledger --tenant <id>` checks the tenant's audit chain and exits non-zero on the first mismatch.

### Environment variables

| Variable | Read by | Meaning |
| --- | --- | --- |
| `POSTGRES_DSN` | `cmd/worker`, `cmd/controlplane` | Postgres connection string. Required. Compose uses `sslmode=verify-full` and `sslrootcert=/certs/ca.crt`. |
| `REDIS_ADDR` | `cmd/worker`, `cmd/controlplane` | Redis host and port. Defaults to `127.0.0.1:6379`. |
| `REDIS_PASSWORD` | `cmd/worker`, `cmd/controlplane` | Redis `AUTH` password. |
| `REDIS_TLS` | `cmd/worker`, `cmd/controlplane` | Set to `1` to connect over TLS. |
| `REDIS_CA` | `cmd/worker`, `cmd/controlplane` | Path to the CA certificate that signed the Redis server certificate. Required when `REDIS_TLS=1`. |
| `SIGNING_KEY` | `cmd/worker`, `cmd/verify` | Base64 of a 64-byte Ed25519 private key. The worker signs every job output and every purge receipt with this key. |
| `CONSUMER` | `cmd/worker` | Consumer name inside the `worker-group` Redis consumer group. Two worker processes must use different names. If this and `-consumer` are both unset, the worker uses the machine hostname. |
| `VAULT_ADDR`, `VAULT_TOKEN` | `cmd/worker`, `cmd/controlplane` | Required. Each job's AES key is wrapped through Vault Transit at `$VAULT_ADDR/v1/transit/{encrypt,decrypt}/coldharbour`. Compose starts Vault in development mode and creates the `coldharbour` transit key. |

### Repository layout

| Path | Contents |
| --- | --- |
| `cmd/worker` | The worker process. Reads `coldharbour:jobs`, runs the registered job type, signs and journals the result, then purges the key. Serves Prometheus metrics on port 9100. Compose does not publish that port on the host, so more than one worker can run. Scrape `http://<worker-container>:9100/metrics` on the Compose network. |
| `cmd/verify` | A standalone checker. `verify output --body --job-id --tenant-id --sig --pub` checks a signed job result. `verify receipt --job-id --purged-at --sig --pub` checks a purge receipt. Neither subcommand touches Redis or Postgres. |
| `internal/redact` | The `redact` job type: the three regular expressions and the `RedactPII` function they implement. |
| `internal/mask` | The `mask` job type: replaces named fields in a JSON document. Added to prove the job-type registry needs no changes to add a job. |
| `internal/runner` | The `JobRunner` interface and the registry. The registry is the only list of job types. |
| `internal/queue` | The Redis stream, the consumer group, the dead-letter queue, and the encrypted checkpoint hash. A new job's `input` field is AES-GCM ciphertext. An entry with no `tenant_id` goes to the dead-letter stream and is not journaled. An entry with no `job_keys` row is buried with reason `input key missing`. An entry whose key exists but whose input does not open is buried. A job that reaches `COMPLETED` or `FAILED` leaves no `input` field on `coldharbour:jobs` or `coldharbour:jobs:dlq`. |
| `internal/seal` | AES-GCM encryption of the checkpoint, Ed25519 signing, the key store, the purge receipt store, and the optional Vault wrapping. |
| `internal/journal` | The Postgres audit row: one row per job with its checksum, its signature, and its final state. |
| `internal/events` | The Redis Pub/Sub event a job publishes on each state transition. |
| `cmd/controlplane` | The HTTP API. `POST /v1/jobs`, `GET /v1/jobs`, `GET /v1/jobs/{id}`, delivery links, the compliance report, and the WebSocket relay. |
| `dashboard` | A React and TypeScript page that drives the API above: submit a job, watch it live, download the result, generate a link, and download the report. |
| `docker-compose.yml` | Local Postgres and Redis, both with a password and TLS. |
| `docker/init.sql` | The schema and the two seeded tenants. |

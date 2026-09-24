# Cold Harbour
[![CI](https://github.com/deakR/cold-harbour/actions/workflows/ci.yml/badge.svg)](https://github.com/deakR/cold-harbour/actions/workflows/ci.yml)

Cold Harbour is an HTTP API and a set of local tools for redacting personal data in text and log lines and recording verifiable processing results. A tenant posts text to `POST /v1/jobs`. The `redact` job uses policy-selected detectors and replaces matches with placeholders. The `mask` job masks named fields in a JSON document. The worker signs completed output, removes the encrypted queued input, destroys the per-job key, and writes a signed purge receipt. A tenant can download a compliance CSV and verify the recorded signatures and audit chain.

The repository contains these runtime and operator components:

- `cmd/controlplane` serves the HTTP API, applies database migrations, and relays job events.
- `cmd/worker` consumes queued jobs, runs the `redact` and `mask` runners, signs results, and records purge receipts.
- `cmd/sentinel` masks log lines before they are written and can hand long lines to the API.
- `cmd/coldharbour` provides migration and tenant administration commands.
- `cmd/verify` checks signed outputs, purge receipts, and tenant audit chains.
- `cmd/sentinel-load` runs the detector load and performance gate.
- `dashboard/` is the React and TypeScript operator and tenant interface.

`internal/detect` implements email, US phone, SSN, Indian phone, Aadhaar, and PAN detection. Aadhaar matches must pass the Verhoeff check. The default `redact` policy uses email, US phone, and SSN. Sentinel enables all six detectors by default.

## Status

This is a work in progress, not a deployed service.

## Run it locally

You need Docker, Go 1.25.1, and Node.js 22 for the dashboard. The certificate script needs Bash, so use Git Bash or WSL on Windows.

1. Generate the local CA and server certificates.

   ```powershell
   bash scripts/gen-certs.sh
   ```

2. Set the passwords and a valid base64-encoded 64-byte Ed25519 private key. The repository does not include a key-generation command.

   ```powershell
   $env:POSTGRES_PASSWORD = "change-me"
   $env:REDIS_PASSWORD = "change-me"
   $env:SIGNING_KEY = "<base64-encoded 64-byte Ed25519 private key>"
   ```

3. Start Postgres, Redis, and Vault. The control plane applies the schema when it starts.

   ```powershell
   docker compose up -d postgres redis vault
   docker compose up vault-init
   ```

4. Set the local process variables in both terminals that will run the control plane and worker.

   ```powershell
   $env:POSTGRES_DSN = "postgres://coldharbour:$env:POSTGRES_PASSWORD@127.0.0.1:5433/coldharbour?sslmode=verify-full&sslrootcert=docker/certs/ca.crt"
   $env:REDIS_ADDR = "127.0.0.1:6380"
   $env:REDIS_PASSWORD = "change-me"
   $env:REDIS_TLS = "1"
   $env:REDIS_CA = "docker/certs/ca.crt"
   $env:VAULT_ADDR = "http://127.0.0.1:8200"
   $env:VAULT_TOKEN = "coldharbour-dev"
   $env:ALLOW_INSECURE_SESSION_COOKIE = "1"
   $env:PORT = "8081"
   ```

   `ALLOW_INSECURE_SESSION_COOKIE=1` is for local HTTP development only.

5. Start the control plane in the first terminal and the worker in the second.

   ```powershell
   go run ./cmd/controlplane
   ```

   ```powershell
   go run ./cmd/worker -consumer worker-1
   ```

6. Create a tenant and keep the printed admin key. The command creates both the tenant and its first admin key.

   ```powershell
   $env:ADMIN_KEY = (go run ./cmd/coldharbour admin create-tenant --name local-admin).Trim()
   ```

   To insert the two development tenant rows instead, run this after the schema exists. It does not create API keys.

   ```powershell
   docker compose --profile dev up seed
   ```

7. Submit a job and read its status.

   ```powershell
   $job = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8081/v1/jobs -Headers @{ "X-API-Key" = $env:ADMIN_KEY } -ContentType "application/json" -Body (@{ input = "Contact jane.doe@example.com or (555) 123-4567."; jobType = "redact" } | ConvertTo-Json)
   $job | ConvertTo-Json
   Invoke-RestMethod -Uri ("http://127.0.0.1:8081/v1/jobs/" + $job.jobId) -Headers @{ "X-API-Key" = $env:ADMIN_KEY }
   ```

8. Start the dashboard.

   ```powershell
   cd dashboard
   npm install
   npm run dev
   ```

   Open the Vite URL and log in with the admin or app key. The browser calls `/v1/session/login`; the control plane sets a session cookie and returns a CSRF token. The API key is not placed in the WebSocket URL. Vite proxies `/v1` and `/d` to `http://127.0.0.1:8081`, as configured in `dashboard/vite.config.ts`.

## Run the Sentinel profile

Stop the manually started control plane and worker first. Set the passwords, `SIGNING_KEY`, and certificates as above, then run:

```powershell
docker compose --profile sentinel up --build
```

The profile starts Postgres, Redis, Vault, the control plane, the worker, an example logger, and Sentinel. The logger writes demo addresses to a shared volume. Sentinel masks the lines and writes them to `/logs/app.masked.log`. A one-shot bootstrap creates or reuses the `sentinel-demo` tenant and writes its Sentinel key to `/keys/sentinel.key`.

Read the masked log with:

```powershell
docker run --rm -v coldharbour_sentinel-logs:/logs busybox:1.37 cat /logs/app.masked.log
```

Read the Sentinel key with:

```powershell
docker run --rm -v coldharbour_sentinel-keys:/keys busybox:1.37 cat /keys/sentinel.key
```

Use an admin or app key to log into the dashboard and open the Sentinel view. The key in `/keys/sentinel.key` is for the Sentinel service and is not a dashboard login key.

## Reference

### Endpoints

Most `/v1` routes require an `X-API-Key` or a valid session cookie. `POST /v1/session/login` is public. `GET /d/{token}` is the public delivery route.

| Method and path | What it does |
| --- | --- |
| `POST /v1/session/login` | Accepts `{"apiKey": string}` for an `admin` or `app` key, sets a session cookie, and returns a CSRF token. |
| `GET /v1/session` | Returns the CSRF token for a valid cookie session. |
| `POST /v1/session/logout` | Revokes the current cookie session and clears the cookie. |
| `POST /v1/jobs` | Queues a job. The body is `{"input": string, "jobType": string, "source": string}`. `jobType` defaults to `redact` and must be registered. `source` is optional and accepts `cold-harbour` or `sentinel`. The control plane encrypts `input` with a per-job AES-256-GCM key before writing Redis. Returns `{"jobId", "status": "QUEUED"}`. |
| `GET /v1/jobs` | Lists the calling tenant's jobs, most recent first. |
| `GET /v1/jobs/{id}` | Returns job status. A completed response includes the result, signature, and signing key ID. |
| `POST /v1/jobs/{id}/delivery-links` | Creates a link after the job has an output. The body is `{"expiresAt": RFC3339 string, "maxViews": string from "1" to "100"}`. The expiry must be in the future and no more than 30 days away. Returns the raw token once. |
| `GET /d/{token}` | Resolves a delivery token, checks its expiry and view count, and returns the result. No API key is required. |
| `GET /v1/reports/compliance?from=&to=` | Returns a CSV for the calling tenant. The columns are `dispatch_time,completion_time,final_state,checksum,signature_status,purge_timestamp,source`. |
| `GET /v1/policy` | Returns the tenant policy and an `ETag`. `If-None-Match` can return `304`. The default detectors are `email`, `phone_us`, and `ssn`. |
| `PUT /v1/policy` | Admin only. Accepts `detectors`, `mode` (`redact` or `partial`), `failPolicy` (`closed`, `inline`, or `open`), and `maxInlineBytes`. A successful change appends a `policy_changed` ledger entry. |
| `GET /v1/keys` | Admin only. Lists the tenant's API keys. |
| `POST /v1/keys` | Admin only. Creates a key with `{"name": string, "role": "admin" or "app" or "sentinel"}`. |
| `DELETE /v1/keys/{id}` | Admin only. Revokes a key. The next request with it returns `401`. |
| `GET /v1/sentinel/nodes` | Lists Sentinel node status for the tenant. |
| `POST /v1/sentinel/events` | Sentinel only. Accepts a batch of node counters. |
| `GET /v1/ws/events` | WebSocket relay for the authenticated tenant. Cookie sessions and API keys are accepted. The default allowed browser origins are `localhost:5173` and `127.0.0.1:5173`. |

A tenant that requests a job it did not create receives `404`, the same as for a job that does not exist.

### Keys and roles

`go run ./cmd/coldharbour admin create-tenant --name paykochi` creates a tenant and prints one admin key. Use that key to call `POST /v1/keys` for an `app` or `sentinel` key. The dashboard session login accepts `admin` and `app` keys only. `sentinel` keys are for the Sentinel process.

`go run ./cmd/verify ledger --tenant <id>` checks a tenant's audit chain and exits non-zero on the first mismatch. It requires `POSTGRES_DSN`.

### Environment variables

| Variable | Read by | Meaning |
| --- | --- | --- |
| `POSTGRES_DSN` | `cmd/controlplane`, `cmd/worker`, `cmd/coldharbour`, `cmd/verify` | Postgres connection string. Required by the control plane, worker, migration and tenant commands, and ledger verification. |
| `REDIS_ADDR` | `cmd/worker`, `cmd/controlplane` | Redis host and port. Defaults to `127.0.0.1:6379`; local Compose exposes Redis on host port `6380`. |
| `REDIS_PASSWORD` | `cmd/worker`, `cmd/controlplane` | Redis `AUTH` password. |
| `REDIS_TLS` | `cmd/worker`, `cmd/controlplane` | Set to `1` to connect over TLS. |
| `REDIS_CA` | `cmd/worker`, `cmd/controlplane` | CA certificate path for the Redis server certificate. |
| `SIGNING_KEY` | `cmd/worker` | Base64 of a 64-byte Ed25519 private key. The worker signs outputs and purge receipts. |
| `VAULT_ADDR`, `VAULT_TOKEN` | `cmd/worker`, `cmd/controlplane` | Required Vault connection settings. Job keys are wrapped through Vault Transit. |
| `PORT` | `cmd/controlplane` | HTTP listen port. Defaults to `8080`; the local instructions use `8081`. |
| `WS_ORIGIN_PATTERNS` | `cmd/controlplane` | Comma-separated allowed WebSocket origins. Empty uses the local defaults. |
| `ALLOW_INSECURE_SESSION_COOKIE` | `cmd/controlplane` | Set to `1` for local HTTP development with a non-Secure session cookie. |
| `CONSUMER` | `cmd/worker` | Redis consumer name. The `-consumer` flag takes precedence; otherwise the worker uses the hostname. |
| `SENTINEL_API_KEY` or `SENTINEL_API_KEY_FILE` | `cmd/sentinel` | Sentinel API key, or a path to a file containing it. |
| `WRITE_PROFILE` | `cmd/sentinel-load` | Set to `1` to write `bench/cpu.pprof` and `bench/alloc.pprof`. |

### Repository layout

| Path | Contents |
| --- | --- |
| `cmd/controlplane` | HTTP API, database migrations, authentication, job routes, and WebSocket events. |
| `cmd/worker` | Redis consumer group, job runners, signing, journal writes, purge receipts, and worker metrics. |
| `cmd/sentinel` | Log masker, policy polling, long-line handoff, event batches, and Sentinel metrics. |
| `cmd/coldharbour` | Goose migrations, tenant creation, and Sentinel key bootstrap. |
| `cmd/verify` | Offline output and receipt checks, plus online tenant ledger checks. |
| `cmd/sentinel-load` | 100,000-line detector benchmark and the `bench/baseline.json` performance gate. |
| `internal/detect` | Email, US phone, SSN, Indian phone, Aadhaar, and PAN detection. |
| `internal/redact` | The `redact` job runner and result format. |
| `internal/mask` | The `mask` job runner for named JSON fields. |
| `internal/queue` | Redis streams, consumer groups, retries, dead letters, checkpoints, and purge handling. |
| `internal/seal` | AES-GCM encryption, Ed25519 signing, key storage, Vault wrapping, and receipts. |
| `internal/journal` | Durable job rows, checksums, outputs, and state history. |
| `internal/ledger` | Per-tenant append-only hash chain and verification. |
| `internal/api` | Authentication, jobs, policies, keys, delivery links, reports, sessions, and events. |
| `dashboard` | React and TypeScript UI for jobs, results, delivery links, reports, and Sentinel nodes. |
| `docker-compose.yml` | Local Postgres, Redis, Vault, worker, control plane, and profile services. |
| `docker/init.sql` | A no-op `SELECT 1`. The control plane applies the schema through Goose migrations. |
| `dev/seed.sql` | Optional development tenant rows. It does not create API keys. |
| `scripts/gen-certs.sh` | Generates the local CA and server certificates. |
| `.github/workflows/ci.yml` | Test, contract, build, security scan, and performance checks. |

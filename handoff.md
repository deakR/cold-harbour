# ColdHarbor — Handoff

Date: 2026-09-04T03:05:00Z
Branch: `main`
Constraint: local Git only. No `push`, no remote modifications. Remote `origin` exists (github.com/deakR/cold-harbour.git) — do not use.

## 1. State

Platform milestones M1-M4 plus polish batch (auth, observability, DLQ API +
redrive, OpenAPI, rate limiting, full-stack Docker, UI auth/chaos/TTL badge)
implemented, committed locally, and proven against the live stack.

SaaS: `apps/batchseal/` Wails desktop app (Go stdlib backend + React) with
dispatch, live polling, local SHA-256 seal verification, offline receipts,
audit browser, fleet view, and DLQ redrive. Binary built at
`apps/batchseal/build/bin/batchseal.exe` (gitignored).

Live stack (running): Redis 7.2 `:6379`, Postgres 16 on host `:5433`
(host 5432 occupied by a local postgres process), control plane `:8080`
(`DB_PORT=5433`), Go worker with metrics `:9091`.

Live evidence: E2E 6/6 PASS, 50-job benchmark (avg 12.7ms, 10/10 sealed),
crash-recovery chaos (failed 1, recovered 1), DLQ redrive loop
(DLQ 1 → redriven 1 → DLQ 1 on poison), exactly 1 audit row per compartment.
Details in `docs/benchmarks.md`.

## 2. Verification

- `go test ./...` (worker-engine): PASS. `go vet` clean.
- `go test ./backend/` (batchseal, httptest fake): PASS, stdlib only.
- `mvn -B test`: 41/41 PASS.
- `npm test` (vitest): 12/12 PASS. `npm run build` clean (both web and batchseal frontend).
- `verify_e2e.ps1 -MockMode` and live (`-PostgresPort 5433`): 6/6 PASS.
- No Python dependency. CI defined in `.github/workflows/verify.yml` (local file only, never pushed).

## 3. Remaining work

1. Record the 60-90s demo (dashboard + BatchSeal chaos probe + metrics).
2. Push and watch CI green (needs explicit approval under the local-only rule).
3. Optional: multi-worker 1,000-job scale run not yet attempted.
4. `ORIGINAL_REQUEST.md` acceptance checkboxes still unchecked; check them after reviewing this handoff.

## 4. Resume commands

```powershell
git status --short
go test ./...            # from services/worker-engine
go test ./backend/       # from apps/batchseal
powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1 -MockMode
docker compose -f docker/docker-compose.yml up -d
$env:DB_PORT='5433'; java -jar services/control-plane/target/control-plane-1.0.0-SNAPSHOT.jar
go run ./cmd/worker      # from services/worker-engine
```

## 5. Notes

- `job.go` extensions are backward-compatible (`omitempty`). Do not revert.
- `miniredis`/`gopher-lua` are test-only indirect deps. Do not promote to direct deps.
- `docs/contracts.md` remains authoritative for keyspace and FSM. `PROJECT.md` interface contracts match implementation.
- Frontend `App.css` vs `app.css` lesson: Windows filesystem is case-insensitive; BatchSeal styles live in `batchseal.css`.
- No `push` per `ORIGINAL_REQUEST.md` safety rule. Remote `origin` exists; verify with `git remote -v` before any Git network op.

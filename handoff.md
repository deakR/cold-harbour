# ColdHarbor — Handoff

Date: 2026-09-03T23:00:00Z
Branch: `main` (through `f1d7fe1`)
Constraint: local Git only. No `push`, no remote modifications. Remote `origin` exists (github.com/deakR/cold-harbour.git) — do not use.

## 1. State

All milestones M1-M4 implemented, committed locally, and proven against the live
stack. Working tree clean.

Live stack (running): Redis 7.2.16 `:6379`, Postgres 16 on host `:5433`
(host 5432 occupied by a local postgres process), control plane `:8080`
(`DB_PORT=5433`), Go worker with metrics `:9091`.

Live evidence: E2E 6/6 PASS, 50-job benchmark (avg 12.7ms, p50 11.2, p95 14.2,
10/10 sealed), crash-recovery chaos (failed 1, recovered 1, sealed with correct
`reducedSum`), exactly 1 audit row per compartment. Details in
`docs/benchmarks.md`.

## 2. Verification

- `go test ./...`: PASS. `go vet` clean.
- `mvn -B test`: 33/33 PASS.
- `npm test` (vitest): 12/12 PASS. `npm run build` clean.
- `verify_e2e.ps1 -MockMode`: 6/6 PASS. Live run 6/6 PASS (see `docs/benchmarks.md`).
- No Python dependency. CI defined in `.github/workflows/verify.yml` (local file only, never pushed).

Live infra verification blocked: Docker daemon down (`npipe:////./pipe/dockerDesktopLinuxEngine` unreachable). Mock mode only.

## 3. Remaining work

1. Live verification (requires Docker Desktop running):
   - `docker compose -f docker/docker-compose.yml up -d`
   - `powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1` (no `-MockMode`)
   - If live fails, fix root cause only.
2. Optional full builds (only if touched):
   - `go build ./...`, `go test ./...`
   - `./mvnw -q test` in `services/control-plane`
   - `npm run build` in `web`
3. Update `ORIGINAL_REQUEST.md` acceptance checkboxes only after live pass.

## 4. Resume commands

```powershell
git status --short
go test ./...  # from services/worker-engine
powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1 -MockMode
docker compose -f docker/docker-compose.yml up -d
powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1
```

## 5. Notes

- `job.go` extensions are backward-compatible (`omitempty`). Do not revert.
- `miniredis`/`gopher-lua` are test-only indirect deps. Do not promote to direct deps.
- `docs/contracts.md` remains authoritative for keyspace and FSM. `PROJECT.md` interface contracts match implementation.
- No `push` per `ORIGINAL_REQUEST.md` safety rule. Remote `origin` exists; verify with `git remote -v` before any Git network op.

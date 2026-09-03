# ColdHarbor — Handoff

Date: 2026-09-03T17:16:12Z
Branch: `main` (commits `7f5e53e`, `44b5acf`, `1d6449f`)
Constraint: local Git only. No `push`, no remote modifications.

## 1. State

All milestones M1-M4 are implemented on disk. `PROJECT.md` marks M1-M4 DONE. Git history only contains M1 + partial M2. Remainder is uncommitted.

Committed:
- `docker/docker-compose.yml`, `docker/init-db.sql`, `docs/contracts.md`
- `services/worker-engine/internal/model/{job,state}.go` (base version)
- `services/worker-engine/go.mod`, `go.sum` (base version)

Uncommitted but present and passing:
- `services/worker-engine/cmd/worker/main.go`
- `services/worker-engine/internal/config/config.go` + test
- `services/worker-engine/internal/engine/` (consumer, fsm, scratchpad, checkpoint, recovery, archive, heartbeat, events, dlq, runner + 9 test files including `adversarial_challenge_test.go`, `challenger_m1_deep_test.go`)
- `services/worker-engine/internal/model/state_test.go`
- `services/control-plane/` full Spring Boot app (controllers, services, security, websocket, entity, dto, tests). Built artifact exists: `target/control-plane-1.0.0-SNAPSHOT.jar`
- `web/` full Vite+React+TS app (WorkerTelemetry, LifecycleTracker, EventFeed, DeadDropInspector, AuditBrowser, JobDispatcher, hooks, utils). Built artifact exists: `dist/`
- `scripts/verify_e2e.ps1`, `tests/e2e/test_engine.py`, `tests/e2e/fixtures/*.json`
- Root docs: `ORIGINAL_REQUEST.md`, `PROJECT.md`, `TEST_INFRA.md`, `TEST_READY.md`

Modified (tracked):
- `services/worker-engine/go.mod`: added `miniredis/v2 v2.39.0`, `gopher-lua v1.1.1` (test harness)
- `services/worker-engine/go.sum`: matching hashes
- `services/worker-engine/internal/model/job.go`: added `MaxRetries`, `TimeoutSeconds` to `JobMessage`; `PreviousState`, `CurrentState`, `Progress`, optional `Details` to `EventMessage`; `Context`, `TaskType`, `ResultPayload`, `DurationMs`, `CompletedAt` tolerance to `DeadDropPayload`

## 2. Verification (this session, no rework)

- `go test ./...` in `services/worker-engine`: PASS (`config`, `engine`, `model`; `cmd/worker` has no tests)
- `python tests/e2e/test_engine.py`: 6/6 PASS
- `scripts/verify_e2e.ps1 -MockMode`: 6/6 PASS (Scenarios 1-6, 100%)
- `services/control-plane/target/` contains built jar + `surefire-reports/` (prior build succeeded)
- `web/dist/` contains built bundle (prior build succeeded)

Live infra verification (Redis 6379, Postgres 5432, control-plane 8080) was not run this session. Mock mode only.

## 3. Remaining work

1. Stage and commit uncommitted work locally (no push):
   `git add ORIGINAL_REQUEST.md PROJECT.md TEST_INFRA.md TEST_READY.md scripts/ tests/ web/ services/control-plane/ services/worker-engine/cmd/ services/worker-engine/internal/config/ services/worker-engine/internal/engine/ services/worker-engine/internal/model/state_test.go services/worker-engine/go.mod services/worker-engine/go.sum services/worker-engine/internal/model/job.go`
   Then `git commit` with M2/M3/M4 messages. Keep commits local.
2. Live verification:
   - `docker compose -f docker/docker-compose.yml up -d`
   - `powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1` (no `-MockMode`)
   - If live fails, fix root cause only.
3. Optional full builds (only if touched):
   - `go build ./...`, `go test ./...`
   - `./mvnw -q test` in `services/control-plane`
   - `npm run build` in `web`
4. Update `ORIGINAL_REQUEST.md` acceptance checkboxes only after live pass.

## 4. Resume commands

```powershell
git status --short
go test ./...  # from services/worker-engine
python tests/e2e/test_engine.py  # from root
powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1 -MockMode
docker compose -f docker/docker-compose.yml up -d
powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1
```

## 5. Notes

- `job.go` extensions are backward-compatible (`omitempty`). Do not revert.
- `miniredis`/`gopher-lua` are test-only indirect deps. Do not promote to direct deps.
- `docs/contracts.md` remains authoritative for keyspace and FSM. `PROJECT.md` interface contracts match implementation.
- No remote configured for push per `ORIGINAL_REQUEST.md` safety rule. Verify with `git remote -v` before any Git network op.

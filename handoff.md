# ColdHarbor — Handoff

Date: 2026-09-03T17:30:00Z
Branch: `main` (commits `7f5e53e`, `44b5acf`, `1d6449f`, `a9b5c01`, `5897594`, `a128301`, `73482f4`)
Constraint: local Git only. No `push`, no remote modifications. Remote `origin` exists (github.com/deakR/cold-harbour.git) — do not use.

## 1. State

All milestones M1-M4 are implemented and committed locally. `git status --short` is clean. Working tree matches HEAD `73482f4`.

Committed this session:
- `a9b5c01 feat(worker): complete Go engine M1` — `cmd/`, `config/`, `engine/` (consumer, fsm, scratchpad, checkpoint, recovery, archive, heartbeat, events, dlq, runner + tests), `state_test.go`, `job.go` extensions (`MaxRetries`, `TimeoutSeconds`, `PreviousState`/`CurrentState`, `Progress`, `ResultPayload`, `DurationMs`), `miniredis`/`gopher-lua` test deps
- `5897594 feat(control): Spring Boot plane M2` — clearance, REST, stream producer, audit, websocket, wellness + tests
- `a128301 feat(web,verify): React dashboard M3, E2E harness M4, docs and handoff` — `web/src/`, `scripts/verify_e2e.ps1`, `tests/e2e/`, `ORIGINAL_REQUEST.md`, `PROJECT.md`, `TEST_INFRA.md`, `TEST_READY.md`, `handoff.md`
- `73482f4 chore(web): track nested gitignore`
- `.gitignore` extended: `target/`, `*.class`, `*.jar.original`, `web/dist/`, `node_modules/` (prevents build artifact commits; `control-plane/target/` and `web/dist/` remain on disk, untracked)

## 2. Verification (this session, no rework)

- `go test ./...` in `services/worker-engine`: PASS (`config`, `engine`, `model`; `cmd/worker` has no tests)
- `python tests/e2e/test_engine.py`: 6/6 PASS
- `scripts/verify_e2e.ps1 -MockMode`: 6/6 PASS (Scenarios 1-6, 100%)
- `services/control-plane/target/` contains built jar + `surefire-reports/` (prior build succeeded)
- `web/dist/` contains built bundle (prior build succeeded)

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
python tests/e2e/test_engine.py  # from root
powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1 -MockMode
docker compose -f docker/docker-compose.yml up -d
powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1
```

## 5. Notes

- `job.go` extensions are backward-compatible (`omitempty`). Do not revert.
- `miniredis`/`gopher-lua` are test-only indirect deps. Do not promote to direct deps.
- `docs/contracts.md` remains authoritative for keyspace and FSM. `PROJECT.md` interface contracts match implementation.
- No `push` per `ORIGINAL_REQUEST.md` safety rule. Remote `origin` exists; verify with `git remote -v` before any Git network op.

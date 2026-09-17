# Verification handoff

Checked on 2026-09-17 before repository publication.

## Passed locally

- Go: `go vet ./...` and `go test ./...`.
- Control plane: 41 tests passed; Maven package build passed.
- Dashboard: 12 tests passed; production build passed.
- Offline contract simulation: six scenarios passed.

## Live verification blocked

Running `scripts/verify_e2e.ps1 -PostgresPort 5433` without `-MockMode`
reported the control plane at `localhost:8080` offline. Redis and PostgreSQL
ports were reachable, but the script fell back to contract simulation.
Its successful exit is **not a live integration pass**.

Docker Compose could not connect to the Docker Desktop Linux engine.
Start the full stack and repeat live verification before claiming deployment
readiness. The development credentials, self-asserted clearance, and exposed
service ports are not suitable for direct internet deployment.

## Publication fixes

- Added README status/CI badges and contributor instructions.
- Preserved the existing test-file renames and reference-document cleanup.
- Marked the Unix Maven launcher executable for CI.
- Removed a developer-specific Maven path from the Windows launcher; install
  Maven on PATH before using either launcher.

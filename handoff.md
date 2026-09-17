# Verification handoff

Checked on 2026-09-17 before repository publication.

## Passed locally

- Go: `go vet ./...` and `go test ./...`.
- Control plane: 41 tests passed; Maven package build passed.
- Dashboard: 12 tests passed; production build passed.
- Offline contract simulation: six scenarios passed.

## Live verification performed on 2026-09-17

The full Docker Compose stack built and started successfully. Direct checks
against the API proxy, Redis, worker logs, and PostgreSQL established:

- Dispatch through `localhost:3000/api/v1/compartments` returned HTTP 201.
- `cpt_fdd700e01237`: reduction completed, Redis scratchpad `EXISTS` returned
  0, and PostgreSQL contained a `PURGED` row with the matching checksum.
- An independent SHA-256 calculation matched the output checksum. Shortening
  this test job's archive TTL to one second caused real Redis expiry; the API
  then served matching output/checksum from durable storage with remaining TTL 0.
- `cpt_758619f82e4c`: observed step 2 / 50% scratchpad and a pending stream
  entry using the injected crash flag. Killed the original worker with SIGKILL
  and started `worker-verification-successor`. The successor completed the job;
  pending count became 0, the scratchpad was purged, and the audit row persisted.
  This verifies recovery after an injected checkpoint interruption, not arbitrary
  process failure timing. The temporary successor was removed and the normal
  worker restarted afterward.
- `cpt_4503ef625861` (CIPHER_STREAM) and `cpt_111838983f56` (ARCHIVE_SEAL)
  produced task-specific output and matching durable `PURGED` records.
- `cpt_298f4d79785e`: injected failure reached the DLQ after three attempts
  and produced a `FAILED` audit row with an empty checksum.
- DLQ redrive returned HTTP 200. It selected the pre-existing oldest entry,
  `cpt_baaca3f9598a`, rather than the new verification entry. That poison pill
  was retried and returned to the DLQ. Test records remain in local storage.
- Dashboard HTML returned HTTP 200. Worker `/metrics` exposed counters and
  `/healthz` returned UP. Direct control-plane `/actuator/health` returned UP
  with database and Redis checks.

## Limitations and follow-up

- `scripts/verify_e2e.ps1` is not a complete live E2E harness: scenarios 2–6
  simulate state even when infrastructure is available. Its scorecard must not
  be cited as live verification. Use `node scripts/verify_live.mjs` (Makefile
  `verify-live`) for a repeatable real HTTP/WebSocket smoke test; it creates
  one job, verifies proxied health, Prometheus metrics, WebSocket delivery
  including PURGED, output checksum, and archive TTL, and fails without services.
- The attempted proxied actuator checks initially used the wrong
  `/api/actuator/...` path; the configured path `/actuator/...` was verified
  live later (see follow-up section).
- DATA_REDUCTION performs a demonstration calculation based on `batchSize`;
  it does not sum the supplied `inputValues`.
- `npm audit` initially reported five development/tooling findings involving
  Vite/Vitest; resolved by the tooling upgrade recorded below.

## Follow-up checks completed on 2026-09-17

- Upgraded `web` dev tooling to Vite 7.3.6, Vitest 4.1.11, and
  `@vitejs/plugin-react` 5.2.0. All 12 frontend tests pass, the production
  build succeeds, and `npm audit` reports zero vulnerabilities including dev
  dependencies. The rebuilt `coldharbor/web` image serves the same bundle size
  (61.03 KB gzip) and the live smoke test passes against it.
- `node scripts/verify_live.mjs` passed repeatedly against the running stack:
  dashboard HTML, proxied `/actuator/health` (UP), `/actuator/prometheus`
  metrics, HTTP 201 dispatch, ten WebSocket job events including `PURGED`,
  output checksum, and archive TTL.
- Real WebSocket `ping` -> `pong` round trip verified through nginx and the
  control plane.
- Headless Edge (Playwright core, no repo dependency) rendered the dashboard,
  dispatched `cpt_4bd341334941` via the UI form (HTTP 201), opened Live Events,
  and displayed the new job ID with no page errors. Interactions other than
  dispatch were not exercised.
- The earlier Docker outage was transient; the full stack is running again and
  the follow-up checks above were completed against it.

## Publication fixes

- Added README status/CI badges and contributor instructions.
- Preserved the existing test-file renames and reference-document cleanup.
- Marked the Unix Maven launcher executable for CI.
- Removed a developer-specific Maven path from the Windows launcher; install
  Maven on PATH before using either launcher.

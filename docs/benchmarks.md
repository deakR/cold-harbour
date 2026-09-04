# ColdHarbor Benchmarks

Method: `scripts/benchmark.ps1` dispatches N jobs of mixed task types
(`DATA_REDUCTION`, `CIPHER_STREAM`, `ARCHIVE_SEAL`) to the control plane REST
API and reports dispatch latency. When a Go worker is live, it polls the first
10 Dead Drops for up to 60s to report completion rate. Chaos procedure: start
two workers, dispatch with `simulateCrashAtStep: 2` in the payload, kill the
claiming worker process, and confirm the successor seals the job from step 3
via `XAUTOCLAIM` (see `TEST_INFRA.md` TC-T1-F07-01).

## Results

| Date (UTC) | Jobs | Dispatch avg/p50/p95/max (ms) | Sealed | Notes |
|---|---|---|---|---|
| 2026-09-03 17:16 | 50 mixed (`DATA_REDUCTION`/`CIPHER_STREAM`/`ARCHIVE_SEAL`) | 12.7/11.2/14.2/74.3 | 10/10 probed | Worker counters: claimed 51, completed 51, sealed 51, purged 51, failed 0, dlq 0. Full audit rows in `audit_records` with output + checksum. |

## Chaos

| Date (UTC) | Scenario | Outcome |
|---|---|---|
| 2026-09-03 17:19 | `simulateCrashAtStep: 2`, single worker, PEL reclaim | Crashed at step 2, sealed 43s later from step 3 (`reducedSum` 19801 correct). Metrics: failed 1, recovered 1, completed 52. Exactly 1 audit row after single-thread relay fix. |
| 2026-09-04 03:00 | Poison pill (`simulateFailure`, `maxRetries: 1`) then DLQ redrive | Failed job landed in `coldharbor:jobs:dlq` with reason and original payload. `POST /api/v1/workers/dlq/redrive` returned `redriven: 1`; worker reprocessed and the still-failing job returned to the DLQ (size back to 1). Redrive loop proven end to end. |

## Live E2E

| Date (UTC) | Mode | Result |
|---|---|---|
| 2026-09-03 | Mock (`verify_e2e.ps1 -MockMode`) | 6/6 PASS |
| 2026-09-03 17:15 | Live (Redis 7.2.16 + Postgres 16 + control plane + Go worker) | 6/6 PASS after fixing harness header (`X-Context` → `X-Context-Clearance`) |

Note: host PostgreSQL occupies port 5432 on the dev machine, so
`docker/docker-compose.yml` maps Postgres to host port 5433. Start the control
plane with `DB_PORT=5433` and pass `-PostgresPort 5433` to `verify_e2e.ps1`.

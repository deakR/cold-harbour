# ColdHarbor

[![Verify](https://github.com/deakR/cold-harbour/actions/workflows/verify.yml/badge.svg)](https://github.com/deakR/cold-harbour/actions/workflows/verify.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> A distributed, compartmentalized task execution platform.

ColdHarbor demonstrates asynchronous workload execution with explicit context
boundaries, checkpoint-based recovery, ephemeral working state, and durable
execution records.

---

## Overview

Each job runs in a **Compartment** with an explicit security context
(`INNIE` or `OUTIE`). Intermediate state is stored temporarily in Redis and
removed at terminal completion. Final output is sealed with a SHA-256 checksum,
retained for a configurable TTL, and represented in PostgreSQL.

ColdHarbor is intentionally an educational reference implementation. It does
not provide an OS sandbox for arbitrary untrusted code.

---

## High-Level Architecture

```
                        Client
                          │
                  React Dashboard
                          │
                 REST + WebSockets
                          │
                 Spring Boot API
                          │
      ┌───────────────────┼───────────────────┐
      │                   │                   │
 Context Proxy       Scheduler        Audit Service
      │                                       │
Compartment Manager                           ▼
      │                                 PostgreSQL
Redis Streams (Job Queue)               (Durable Audit Log)
      │
──────────────────────────────────────────────
            Go Worker Cluster
──────────────────────────────────────────────
      │
      ├── Worker Pool (Goroutines)
      ├── Finite State Machine
      ├── Ephemeral Memory Manager (Hashes)
      ├── Checkpoint Engine
      └── Dead Drop Archive (SHA-256 + TTL)
      │
Redis (Streams / Hashes / Pub/Sub / TTL)
```

---

## Tech Stack & Separation of Concerns

- **Spring Boot 3 (Java 21)**: Control plane, API-key clearance mapping, context checks at the API layer, compartment lifecycle manager, REST endpoints, and WebSocket event gateway.
- **Go (Golang 1.25+)**: High-throughput worker engine, Redis Stream consumer groups, deterministic state machine, checkpoint recovery, and ephemeral memory management.
- **Redis 7 (Alpine)**: Execution backbone powering the job stream
  (`coldharbor:jobs`), durable lifecycle and audit streams, ephemeral memory
  hashes (`compartment:{id}:mem`), live pub/sub broadcasts
  (`coldharbor:events`), and TTL dead drops (`archive:{id}`).
- **PostgreSQL 16 (Alpine)**: Relational, durable audit store preserving historical run records, execution durations, and output checksums.
- **React (Vite + TypeScript)**: Operational dashboard with live worker telemetry, compartment state visualizer, and event feeds.
- **Docker Compose**: Containerized local development and deployment orchestration.

---

## Core Primitives

### 1. Compartment Isolation
Each task is encapsulated in its own execution context. Cross-compartment memory access is blocked at the routing and key-namespace layers.

### 2. Ephemeral Scratchpad Memory
Intermediate state is held in Redis Hashes (`compartment:{id}:mem`). Terminal cleanup deletes the scratchpad key using `DEL`. This is logical deletion, not secure erasure: Redis persistence files, backups, and process memory may retain data.

### 3. Checkpointing & Crash Resilience
Workers record periodic progress checkpoints (e.g., 25%, 50%, 75%). If a worker crashes mid-task, another worker claims the unacknowledged message from the Redis Stream consumer group and resumes execution from the latest checkpoint.

### 4. Dead Drop Archive
Completed task outputs are sealed with a **SHA-256 integrity checksum** and stored with an expiring **TTL** in Redis (`archive:{id}`). The checksum detects tampering and corruption; it is not a digital signature and does not authenticate the producer.

### 5. Dual-Tier Persistence
- **Hot / Ephemeral (Redis)**: Milliseconds to hours lifespan for active queues, scratchpads, and dead drops.
- **Cold / Permanent (PostgreSQL)**: Durable `audit_records` table storing completion metadata, timestamps, and integrity checksums.

### 6. Durable Event Delivery

Workers append lifecycle events to `coldharbor:events:stream` and terminal
audit envelopes to `coldharbor:audits`. The control plane consumes these
streams and acknowledges entries only after projection or PostgreSQL
persistence succeeds. Redis Pub/Sub remains a low-latency dashboard fanout
channel; it is not the durable source of truth.

---

## Project Structure

```text
coldharbour/
├── docker/
│   ├── docker-compose.yml     # Redis 7 + PostgreSQL 16 + full-stack services
│   ├── init-db.sql            # Local database bootstrap
│   └── ...                    # Versioned Flyway migrations
├── docs/
│   ├── contracts.md           # Redis schemas, payloads, and state transitions
│   ├── threat-model.md        # Trust boundaries and abuse-case mitigations
│   └── benchmarks.md          # Measured live results (E2E, dispatch, chaos)
├── scripts/
│   ├── verify_e2e.ps1         # 6-scenario E2E harness (live + mock modes)
│   └── benchmark.ps1          # Dispatch benchmark and completion probe
├── services/
│   ├── control-plane/         # Spring Boot 3 API (Java 21): clearance auth,
│   │                          # stream producer, audit, WebSocket relay, wellness
│   └── worker-engine/         # Go worker pool: FSM, checkpoints, XAUTOCLAIM
│                              # recovery, Dead Drop sealing, Prometheus metrics
├── web/                       # React telemetry dashboard (Vite + TypeScript)
├── Makefile                   # build/up/stack/verify/bench shortcuts
└── README.md
```

---

## Project status

The repository contains a working local stack and automated verification for
the control plane, worker engine, dashboard, and contract harness. The default
Compose configuration is for local development only: it publishes database
ports and uses development credentials. Do not expose it directly to the
internet. See the [threat model](docs/threat-model.md) for boundaries and
residual risks.

## Quickstart

### Full stack with Docker Compose

Docker Compose builds the worker, control plane, and dashboard images and
starts Redis and PostgreSQL with their health checks.

```bash
docker compose -f docker/docker-compose.yml up -d --build
```

Open:

- Dashboard: <http://localhost:3000>
- API: <http://localhost:8080>
- Swagger UI: <http://localhost:8080/swagger-ui/index.html>

The Compose stack is configured for local development and accepts the
self-asserted `X-Context-Clearance` header. Use API keys and disable that
development escape hatch before deploying elsewhere.

Stop the stack with:

```bash
docker compose -f docker/docker-compose.yml down
```

### Infrastructure only

```bash
make up
```

Start the worker, control plane, and dashboard from source after the
infrastructure is ready.

| Service | Host port | Credentials / notes |
|---|---|---|
| Redis 7 | `6379` | No auth (dev) |
| PostgreSQL 16 | `5433` | DB `coldharbor`, user `coldharbor`, password `coldharbor_secret`. Host port remapped because dev machines often run Postgres on `5432`; the container still listens on `5432`, so Docker-networked services use `DB_PORT=5432`. |
| Control plane | `8080` | Start with `DB_PORT=5433` when running the jar locally |
| Go worker metrics | `9091` | Prometheus text at `/metrics`, JSON at `/healthz` |
| Dashboard (dev) | `3000` | `npm run dev` in `web/` (proxies `/api` and `/ws` to `:8080`) |

### Prerequisites

- [Docker & Docker Compose](https://www.docker.com/)
- [Go 1.25+](https://go.dev/)
- [Java 21+](https://adoptium.net/) and [Maven](https://maven.apache.org/) installed on `PATH` (`mvnw` is a launcher, not a self-downloading wrapper)
- [Node.js 22.12+](https://nodejs.org/)
- GNU Make for the shortcuts above; Windows PowerShell for the Makefile verification/benchmark targets (or run the scripts directly with PowerShell 7 `pwsh`)

### Verify

```bash
make verify        # deterministic contract verification; no Docker needed
make verify-live   # real HTTP/WebSocket/checksum/TTL smoke test
make bench         # 50-job dispatch benchmark and completion probe
```

`verify_e2e.ps1 -MockMode` is deterministic and simulates worker scenarios.
`verify_live.mjs` requires the running stack and never falls back to
simulation. It creates a verification job and retains its audit record.

---

## Security

ColdHarbor has two clearance mechanisms:

1. `X-API-Key`, mapped server-side to a clearance level. This is the
   production mechanism.
2. `X-Context-Clearance`, accepted only when
   `COLDHARBOR_ALLOW_UNSAFE_HEADER=true`. Compose enables this explicitly for
   local development.

WebSocket handshakes use the `coldharbor` subprotocol plus
`clearance.<LEVEL>`. When API keys are configured, clients also provide an
`api-key.<base64url-key>` subprotocol. Events are filtered by the authenticated
session's clearance.

Before a non-local deployment:

- set `COLDHARBOR_ALLOW_UNSAFE_HEADER=false`;
- configure API keys through a secret manager;
- restrict `COLDHARBOR_ALLOWED_ORIGINS`;
- keep Redis and PostgreSQL on private networks with authentication and TLS;
- restrict Actuator, Swagger, metrics, and DLQ operator endpoints.

Read [SECURITY.md](SECURITY.md) and the
[threat model](docs/threat-model.md) for the deployment baseline.

## API overview

All protected API operations require clearance: `X-API-Key` (preferred, mapped
server-side via `coldharbor.security.api-keys`) or, when
`allow-unsafe-header=true` (explicitly enabled by development Compose), a self-asserted
`X-Context-Clearance: INNIE|OUTIE|SYSTEM|ADMIN` header. Every response carries
`X-Trace-Id`.

| Method & Path | Description |
|---|---|
| `POST /api/v1/compartments` | Dispatch job to `coldharbor:jobs` (rate-limited when `COLDHARBOR_DISPATCH_PER_MINUTE>0`) |
| `GET /api/v1/compartments/{id}` | Lifecycle state and progress (Redis live, Postgres fallback) |
| `GET /api/v1/compartments/{id}/deaddrop` | Verified Dead Drop; falls back to durable audit after TTL expiry (`remainingTtlSeconds: 0`) |
| `GET /api/v1/audits` | Filterable audit history (`compartmentId`, `ownerId`, `context`) |
| `GET /api/v1/workers` | Worker liveness from heartbeat scan |
| `GET /api/v1/workers/dlq` | Dead-letter queue depth + recent entries (`?limit=`) |
| `POST /api/v1/workers/dlq/redrive` | Requeue dead letters onto the main stream (`?limit=`); poison pills that still fail return to the DLQ |
| `GET /ws/events` | WebSocket fanout of `coldharbor:events` transitions |
| `GET /actuator/health`, `/actuator/prometheus` | Liveness and metrics |

Browser WebSocket clients authenticate with the `coldharbor` subprotocol plus
`clearance.<LEVEL>` and, when configured, an `api-key.<base64url-key>`
subprotocol. This keeps API keys out of URLs and proxy access logs.

Full interactive reference: `/swagger-ui/index.html`.

---

## Environment Reference

| Variable | Default | Purpose |
|---|---|---|
| `DB_HOST` / `DB_PORT` / `DB_NAME` | `localhost` / `5432` / `coldharbor` | Control-plane Postgres target |
| `SPRING_DATASOURCE_USERNAME` / `SPRING_DATASOURCE_PASSWORD` | `coldharbor` / `coldharbor_secret` | Postgres credentials |
| `SPRING_DATA_REDIS_HOST` / `SPRING_DATA_REDIS_PORT` | `localhost` / `6379` | Control-plane Redis target |
| `REDIS_ADDR` | `localhost:6379` | Worker Redis target |
| `METRICS_ADDR` | `localhost:9091` | Worker metrics/health bind |
| `COLDHARBOR_ALLOW_UNSAFE_HEADER` | `false` | Accept self-asserted clearance. Compose opts in for local development; production must use API keys. |
| `COLDHARBOR_DISPATCH_PER_MINUTE` | `0` (disabled) | Per-IP dispatch rate limit |
| `EVENT_STREAM_NAME` / `AUDIT_STREAM_NAME` | `coldharbor:events:stream` / `coldharbor:audits` | Replayable lifecycle events and terminal audit delivery |
| `COLDHARBOR_ALLOWED_ORIGINS` | Local dashboard origins | Allowed browser and WebSocket origins |
| `COLDHARBOR_API_DOCS_ENABLED` / `COLDHARBOR_SWAGGER_ENABLED` | `false` | Enable API documentation endpoints |
| `COLDHARBOR_ACTUATOR_ENDPOINTS` | `health` | Actuator endpoints exposed over HTTP |

---

## Documentation

- [System Contracts & Protocol Specification](docs/contracts.md)
- [Threat Model](docs/threat-model.md)
- [Benchmarks & Live Verification](docs/benchmarks.md)
- [Security Policy](SECURITY.md)
- [Contributing Guide](CONTRIBUTING.md)

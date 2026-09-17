# ColdHarbor

[![Verify](https://github.com/deakR/cold-harbour/actions/workflows/verify.yml/badge.svg)](https://github.com/deakR/cold-harbour/actions/workflows/verify.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> **A Distributed, Compartmentalized Task Execution Platform**  
> *Memory is temporary. Results are permanent. Context must never leak.*

---

## Overview

**ColdHarbor** is a distributed execution engine built on the principles of execution isolation, ephemeral memory, fault-tolerant checkpointing, and immutable outcomes. 

Every task runs in an isolated **Compartment** with strict security boundaries (`INNIE` / `OUTIE`). During execution, intermediate calculations are written to temporary scratchpad memory. Upon task completion or failure, the runtime scratchpad key is deleted—leaving only a SHA-256 checksummed output in a self-destructing Dead Drop archive and a durable audit record.

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
- **Redis 7 (Alpine)**: Execution backbone powering Stream consumer groups (`coldharbor:jobs`), ephemeral memory hashes (`compartment:{id}:mem`), real-time pub/sub broadcasts (`coldharbor:events`), and self-destructing dead drops (`archive:{id}`).
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

---

## Project Structure

```text
coldharbour/
├── docker/
│   ├── docker-compose.yml     # Redis 7 + PostgreSQL 16 + full-stack services
│   └── init-db.sql            # Audit table DDL and indices
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

## Project Status

ColdHarbor is a development/reference implementation of a distributed task
execution platform. Compartments provide application-level context separation,
not an OS sandbox for untrusted code. The default Compose configuration is for
local development only: it publishes database ports and uses development
credentials and self-asserted clearance. Do not expose it directly to the internet.
See the [threat model](docs/threat-model.md) for security boundaries and limitations.

## Quickstart

### Option A: Full stack in Docker (recommended)

```bash
make build   # Go + Spring jar + web bundle
make stack   # Redis, Postgres, worker, control plane, dashboard
```

Open the dashboard at <http://localhost:3000>, API at
<http://localhost:8080>, Swagger UI at
<http://localhost:8080/swagger-ui/index.html>.

### Option B: Infrastructure only, services from source

```bash
make up
```

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
make verify        # offline contract verification, no Docker needed
make verify-live   # real HTTP/WebSocket smoke test; no simulation fallback
make bench         # 50-job dispatch benchmark + Dead Drop completion probe
```

---

## API Overview

All mutating reads require clearance: `X-API-Key` (preferred, mapped
server-side via `coldharbor.security.api-keys`) or, when
`allow-unsafe-header=true` (dev default), a self-asserted
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
| `COLDHARBOR_ALLOW_UNSAFE_HEADER` | `true` | Accept self-asserted clearance (set `false` in prod with API keys) |
| `COLDHARBOR_DISPATCH_PER_MINUTE` | `0` (disabled) | Per-IP dispatch rate limit |

---

## Documentation

- [System Contracts & Protocol Specification](docs/contracts.md)
- [Threat Model](docs/threat-model.md)
- [Benchmarks & Live Verification](docs/benchmarks.md)

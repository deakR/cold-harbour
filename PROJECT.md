# Project: ColdHarbor Distributed Compartmentalized Task Execution Platform

## Architecture
ColdHarbor is a distributed, compartmentalized task execution platform with zero-trust context isolation, crash resilience, and an immutable audit trail.
- **Worker Engine (`services/worker-engine`)**: High-throughput Go runtime consuming tasks from Redis Streams (`coldharbor:jobs`), processing through a deterministic finite state machine (`CREATED -> QUEUED -> RUNNING <-> CHECKPOINT -> COMPLETED -> ARCHIVED -> PURGED`), utilizing ephemeral Redis Hash scratchpads (`compartment:{compartmentId}:mem`), multi-step checkpointing (25%, 50%, 75%), crash recovery (`XAUTOCLAIM`), Dead Drop sealing (`archive:{compartmentId}`) with SHA-256 and TTL, 10s heartbeats (`worker:{workerId}:heartbeat`), and event broadcasting on Redis Pub/Sub (`coldharbor:events`). Executes three distinct CPU-bound task types with shared checkpoint/resume mechanics: `DATA_REDUCTION` (numeric reduction), `CIPHER_STREAM` (SHA-256 hash chain over inputs), `ARCHIVE_SEAL` (quartile sealing with seal checksum). Exposes Prometheus counters and `/healthz` on `:9091` (stdlib only).
- **Control Plane (`services/control-plane`)**: Java 21 / Spring Boot 3 API with API-key to clearance mapping (`X-API-Key` takes precedence; self-asserted `X-Context-Clearance` only when `allow-unsafe-header=true`), per-request trace IDs (`X-Trace-Id`), Redis Stream producer, PostgreSQL immutable audit persistence (`audit_records`, full output in `metadata`, exactly one row per compartment via terminal-state guard + serialized relay), WebSocket real-time event streaming relay over a single-threaded ordered dispatcher, and worker wellness monitoring. Serves live Dead Drops from Redis and falls back to the durable audit record after TTL expiry, so results survive self-destruct. Actuator health/info/prometheus endpoints enabled.
- **Telemetry Dashboard (`web`)**: React (Vite + TypeScript + Tailwind CSS) single-page application providing live worker telemetry, visual lifecycle FSM tracker, real-time WebSocket event feed, dead drop output inspector, and historical audit browser.
- **Local Infrastructure & Verification (`docker/`, `scripts/`, `.github/`)**: Redis 7.2 and PostgreSQL 16 containerization (host Postgres remap documented in `docs/benchmarks.md`), PowerShell E2E harness plus dispatch benchmark executing the 6 mandatory integration scenarios, GitHub Actions pipeline (Go vet/test, Maven test, npm build/test, mock E2E), and `docs/threat-model.md`. No Python dependency.

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | Stream Consumer Group | Redis Stream consumer group (`worker-group`) consuming jobs from `coldharbor:jobs` | M1 | ORIGINAL_REQUEST §R1 |
| 2 | Dead-Letter Queue (DLQ) | Automatic dead-letter routing to `coldharbor:jobs:dlq` upon reaching max retry attempts | M1 | ORIGINAL_REQUEST §R1 |
| 3 | Finite State Machine (FSM) | Strict transition enforcement `CREATED -> QUEUED -> RUNNING <-> CHECKPOINT -> COMPLETED -> ARCHIVED -> PURGED` (and `FAILED -> PURGED`) | M1 | ORIGINAL_REQUEST §R1, contracts.md |
| 4 | Ephemeral Scratchpad Memory | Redis Hash storage (`compartment:{compartmentId}:mem`) for transient computation state | M1 | ORIGINAL_REQUEST §R1 |
| 5 | Atomic Scratchpad Purge | Strict atomic `DEL` purge of scratchpad on job completion or failure ensuring zero leak (`EXISTS == 0`) | M1 | ORIGINAL_REQUEST §R1 |
| 6 | Multi-Step Checkpointing | Periodic checkpoint state recording (25%, 50%, 75%) to Redis Hashes | M1 | ORIGINAL_REQUEST §R1 |
| 7 | Crash Recovery Logic | PEL reclamation via `XAUTOCLAIM` / stream message recovery resuming execution from latest checkpoint | M1 | ORIGINAL_REQUEST §R1 |
| 8 | Dead Drop Archive Sealing | Sealed final output storage in `archive:{compartmentId}` with SHA-256 checksum and configurable TTL | M1 | ORIGINAL_REQUEST §R1 |
| 9 | Worker Heartbeat Emitter | Liveness reporting every 10s to `worker:{workerId}:heartbeat` with 30s TTL | M1 | ORIGINAL_REQUEST §R1 |
| 10 | Event Publisher | Transition broadcasting onto Redis Pub/Sub channel `coldharbor:events` | M1 | ORIGINAL_REQUEST §R1 |
| 11 | Context Clearance Proxy | `INNIE` vs `OUTIE` security boundary enforcement on job dispatch and status access | M2 | ORIGINAL_REQUEST §R2 |
| 12 | Compartment REST API | Endpoints for compartment creation, job dispatch, status querying, and audit history | M2 | ORIGINAL_REQUEST §R2 |
| 13 | Redis Stream Producer | Control plane producer publishing structured `JobMessage` JSON to `coldharbor:jobs` | M2 | ORIGINAL_REQUEST §R2 |
| 14 | PostgreSQL Audit Service | Durable persistence of immutable execution records to `audit_records` table | M2 | ORIGINAL_REQUEST §R2 |
| 15 | WebSocket Event Gateway | WebSocket relay streaming real-time events from Redis Pub/Sub `coldharbor:events` to UI clients | M2 | ORIGINAL_REQUEST §R2 |
| 16 | Worker Wellness Monitor | Liveness scanner checking `worker:*:heartbeat` keys and detecting inactive workers | M2 | ORIGINAL_REQUEST §R2 |
| 17 | Worker Telemetry UI | Real-time worker status dashboard showing worker ID, active compartment, and liveness | M3 | ORIGINAL_REQUEST §R3 |
| 18 | Compartment Lifecycle Tracker | Visual state machine tracker showing live transitions and checkpoint progress | M3 | ORIGINAL_REQUEST §R3 |
| 19 | Live WebSocket Event Feed | Streaming event feed displaying real-time system events with status indicators | M3 | ORIGINAL_REQUEST §R3 |
| 20 | Dead Drop Inspector UI | UI for querying sealed results by compartment ID, validating SHA-256 seal and displaying TTL | M3 | ORIGINAL_REQUEST §R3 |
| 21 | Historical Audit Record Browser | Searchable and filterable browser querying PostgreSQL audit logs via REST API | M3 | ORIGINAL_REQUEST §R3 |
| 22 | Local Infrastructure Orchestration | Docker Compose configuration for Redis 7.2 and PostgreSQL 16 with init scripts | M4 | ORIGINAL_REQUEST §R4 |
| 23 | E2E Integration Verification Suite | Automated test harness executing all 6 mandatory end-to-end integration scenarios | M4 | ORIGINAL_REQUEST §R4 |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M1 | Go Worker Engine | `services/worker-engine`: Consumer group, DLQ, FSM, scratchpad, checkpoint/recovery, dead drop, heartbeat, events | none | DONE |
| M2 | Spring Boot Control Plane | `services/control-plane`: Java 21 / Spring Boot 3, Clearance filter, REST API, Stream producer, Postgres audit, WebSocket gateway, Worker monitor | none | DONE |
| M3 | React Telemetry Dashboard | `web`: Vite + React + TypeScript + Tailwind CSS dashboard with telemetry, FSM tracker, event stream, dead drop inspector, audit browser | M2 (for API contract) | DONE |
| M4 | E2E Verification & Hardening | `scripts/` and root: Automated 6-scenario verification script, E2E opaque-box test runner, Tier 1-4 pass & Tier 5 adversarial hardening | M1, M2, M3 | DONE |

## Interface Contracts

### 1. Redis Keyspace Conventions
| Key / Pattern | Redis Type | Purpose | TTL |
|---|---|---|---|
| `coldharbor:jobs` | Stream | Task dispatch queue | None |
| `coldharbor:jobs:dlq` | Stream | Dead-letter queue after max retries | None |
| `compartment:{id}:mem` | Hash | Ephemeral computation scratchpad | Atomic DEL on terminal state |
| `archive:{id}` | String/Hash | Sealed Dead Drop output with SHA-256 | Configurable (e.g. 3600s) |
| `worker:{id}:heartbeat` | String | Worker liveness beacon (updated every 10s) | 30s |
| `coldharbor:events` | Pub/Sub Channel | Real-time state transition broadcast | Ephemeral |

### 2. Stream Job Message (`coldharbor:jobs`)
```json
{
  "compartmentId": "uuid",
  "ownerId": "uuid",
  "context": "INNIE | OUTIE",
  "taskType": "DATA_REDUCTION | CIPHER_STREAM | ARCHIVE_SEAL",
  "payload": "base64-encoded or raw JSON string",
  "maxRetries": 3,
  "timeoutSeconds": 300,
  "createdAt": "ISO-8601 UTC timestamp"
}
```

### 3. Pub/Sub Event Message (`coldharbor:events`)
```json
{
  "eventId": "uuid",
  "compartmentId": "uuid",
  "workerId": "worker-N",
  "previousState": "STATE",
  "currentState": "STATE",
  "progress": 50,
  "timestamp": "ISO-8601 UTC timestamp",
  "details": "optional string or object"
}
```

### 4. Dead Drop Archive (`archive:{compartmentId}`)
```json
{
  "compartmentId": "uuid",
  "ownerId": "uuid",
  "checksum": "sha256(resultPayload)",
  "resultPayload": "data string or JSON",
  "durationMs": 1240,
  "completedAt": "ISO-8601 UTC timestamp"
}
```

### 5. Control Plane REST API Endpoints
- `POST /api/v1/compartments`: Create compartment & dispatch job to stream (`X-Context-Clearance: INNIE|OUTIE`)
- `GET /api/v1/compartments/{id}`: Query compartment state & metadata
- `GET /api/v1/compartments/{id}/deaddrop`: Retrieve verified dead drop result
- `GET /api/v1/audits`: Query historical audit records from PostgreSQL
- `GET /api/v1/workers`: Query active workers & liveness status
- `GET /ws/events`: WebSocket endpoint for live Pub/Sub streaming

### 6. PostgreSQL `audit_records` Schema
Columns: `id UUID PRIMARY KEY`, `compartment_id VARCHAR(64) NOT NULL`, `owner_id VARCHAR(64) NOT NULL`, `context VARCHAR(16) NOT NULL`, `task_type VARCHAR(64) NOT NULL`, `final_state VARCHAR(32) NOT NULL`, `checksum VARCHAR(64) NOT NULL`, `duration_ms BIGINT NOT NULL`, `created_at TIMESTAMPTZ NOT NULL`, `completed_at TIMESTAMPTZ NOT NULL`, `metadata JSONB NOT NULL DEFAULT '{}'::jsonb`.

## Code Layout
```
d:\Web\coldharbour\
├── docker/
│   ├── docker-compose.yml
│   └── init-db.sql
├── docs/
│   └── contracts.md
├── services/
│   ├── worker-engine/
│   │   ├── cmd/worker/main.go
│   │   ├── internal/
│   │   │   ├── model/ (job.go, state.go)
│   │   │   ├── config/ (config.go)
│   │   │   ├── engine/ (consumer.go, fsm.go, scratchpad.go, checkpoint.go, recovery.go, archive.go, heartbeat.go, events.go)
│   │   └── go.mod, go.sum
│   └── control-plane/
│       ├── src/main/java/com/coldharbor/controlplane/
│       │   ├── ControlPlaneApplication.java
│       │   ├── api/ (CompartmentController, AuditController, WorkerController)
│       │   ├── service/ (JobDispatchService, AuditService, WorkerWellnessService)
│       │   ├── model/ (Compartment, AuditRecord, JobMessage, EventMessage)
│       │   ├── security/ (ContextClearanceFilter, NamespaceValidator)
│       │   ├── repository/ (AuditRecordRepository)
│       │   └── websocket/ (WebSocketEventGateway, RedisEventSubscriber)
│       ├── src/main/resources/ (application.yml)
│       └── pom.xml
├── web/
│   ├── src/
│   │   ├── App.tsx, main.tsx
│   │   ├── components/ (WorkerTelemetry, LifecycleTracker, EventFeed, DeadDropInspector, AuditBrowser)
│   │   ├── hooks/ (useWebSocketEvents.ts, useApi.ts)
│   │   └── types/ (index.ts)
│   ├── index.html, package.json, vite.config.ts, tsconfig.json, tailwind.config.js
└── scripts/
    └── verify_e2e.ps1
```

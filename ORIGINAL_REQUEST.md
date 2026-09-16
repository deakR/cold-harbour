# Original User Request

## 2026-09-03T09:40:27Z

Implement and deliver the complete ColdHarbor distributed, compartmentalized task execution platform according to the specifications in `context.md`, `README.md`, and `docs/contracts.md`, creating a robust, professional codebase with full test coverage and verification. Do not perform any remote GitHub operations or touch GitHub remotes/CI.

Working directory: d:/Web/coldharbour
Integrity mode: development

## Requirements

### R1. Go Worker Engine Completion (`services/worker-engine`)
Implement the high-throughput Go worker engine with:
- Redis Stream consumer group (`worker-group`) consuming jobs from `coldharbor:jobs` with dead-letter queue (`coldharbor:jobs:dlq`) handling after max retries.
- Finite state machine enforcing transitions: `CREATED -> QUEUED -> RUNNING <-> CHECKPOINT -> COMPLETED -> ARCHIVED -> PURGED` (and `FAILED -> PURGED`).
- Ephemeral scratchpad memory manager storing intermediate state in Redis Hashes (`compartment:{compartmentId}:mem`), strictly purged via atomic `DEL` upon completion or failure.
- Checkpointing engine recording intermediate steps (e.g. 25%, 50%, 75%) with crash-recovery logic that reclaims unacknowledged stream messages and resumes execution from the latest checkpoint.
- Dead Drop archive storing sealed final results in `archive:{compartmentId}` with SHA-256 checksum and configurable TTL.
- Worker heartbeat emitter reporting status every 10s to `worker:{workerId}:heartbeat` with 30s TTL.
- Event publisher broadcasting state transitions to Redis Pub/Sub channel `coldharbor:events`.

### R2. Spring Boot 3 Control Plane (`services/control-plane`)
Implement the Java 21 / Spring Boot 3 control plane with:
- Context isolation & clearance proxy enforcing `INNIE` vs `OUTIE` namespaces.
- REST API for compartment creation, job dispatch, status querying, and audit history.
- Redis Stream producer publishing structured job messages to `coldharbor:jobs`.
- PostgreSQL Audit Service recording immutable execution records in `audit_records` (UUID, compartment ID, owner, context, final state, checksum, duration, timestamps, JSONB metadata).
- WebSocket event gateway subscribing to Redis Pub/Sub `coldharbor:events` and streaming real-time updates to connected clients.
- Worker wellness monitor checking active worker heartbeat keys in Redis and flagging dead workers.

### R3. React Telemetry Dashboard (`web`)
Implement a production-grade React (Vite + TypeScript + Tailwind CSS) dashboard featuring:
- Live worker telemetry status (ID, active compartment, liveness).
- Visual compartment lifecycle state machine tracker.
- Real-time WebSocket event feed streaming live transitions from `coldharbor:events`.
- Dead drop inspection interface allowing retrieval of verified outputs by compartment ID.
- Historical audit record browser querying PostgreSQL audit logs via the control plane REST API.

### R4. Local Infrastructure & Verification Pipeline
- Maintain `docker/docker-compose.yml` for local Redis 7 and PostgreSQL 16 services.
- Provide end-to-end integration verification script demonstrating:
  1. Job submission via REST API.
  2. Worker claiming job, writing scratchpad, and emitting checkpoints.
  3. Worker crash simulation and subsequent checkpoint recovery.
  4. Dead Drop sealing with SHA-256 and TTL verification.
  5. Atomic scratchpad deletion (`DEL`) ensuring zero memory leak.
  6. Durable audit trail persistence in PostgreSQL.
- Maintain documentation integrity and local Git safety: strictly local operations, no remote GitHub pushes or modifications.

## Acceptance Criteria

### Worker Engine & State Machine
- [x] Go worker engine compiles cleanly (`go build ./...`) and passes all unit and integration tests (`go test -v ./...`).
- [x] Consumer group consumes jobs reliably and acknowledges (`XACK`) messages only after successful archive and purge.
- [x] Ephemeral scratchpad key `compartment:{id}:mem` is verified non-existent (`EXISTS == 0`) after completion and after failure.
- [x] Checkpoint resumption successfully recovers interrupted jobs from the exact checkpoint step without re-executing completed work.
- [x] Dead Drop key `archive:{id}` contains valid SHA-256 checksum and expires after configured TTL.

### Control Plane & Persistence
- [x] Spring Boot 3 control plane compiles and packages cleanly with Maven/Gradle.
- [x] REST API validates payload schemas, authenticates requests, and correctly writes jobs to Redis Streams.
- [x] Audit records are durably persisted to PostgreSQL `audit_records` with accurate duration and checksum matching the Dead Drop.
- [x] WebSocket gateway streams events to subscribers with sub-second latency.

### Front-End Dashboard
- [x] React application builds without TypeScript or bundle errors (`npm run build`).
- [x] UI connects to WebSocket and reflects live state changes in real time.
- [x] UI displays worker liveness, compartment progress, and historical audits.

### Safety & Environmental Constraints
- [x] No remote Git operations performed (`git push`, GitHub remote modifications are strictly avoided).

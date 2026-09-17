# ColdHarbor Inter-Service Contracts & Protocol Specification

This document defines the strict communication and storage contracts between the **Spring Boot Control Plane** and the **Go Worker Engine**.

---

## 1. Redis Keyspace Conventions

| Resource | Key Pattern | Type | TTL | Description |
|---|---|---|---|---|
| **Job Stream Queue** | `coldharbor:jobs` | Stream | Persistent | Primary FIFO job queue consumed by Go worker consumer group `worker-group`. |
| **Dead Letter Stream** | `coldharbor:jobs:dlq` | Stream | Persistent | Unprocessable or poison-pill jobs after max retries. |
| **Ephemeral Scratchpad** | `compartment:{compartmentId}:mem` | Hash | Job Lifetime | Temporary intermediate calculations. Deleted on job completion. |
| **Dead Drop Archive** | `archive:{compartmentId}` | String (JSON) | Configurable (Default: 3600s) | Immutable sealed output with SHA-256 checksum and auto-purge countdown. |
| **Worker Heartbeat** | `worker:{workerId}:heartbeat` | String (JSON) | 30s | Liveness pulse emitted every 10s by each active Go worker. |
| **Live Event Bus** | `coldharbor:events` | Pub/Sub | Instantaneous | Real-time state transition broadcast channel. |
| **Durable Event Log** | `coldharbor:events:stream` | Stream | Capped | Replayable lifecycle projection source; acknowledged after control-plane metadata projection. |
| **Durable Audit Queue** | `coldharbor:audits` | Stream | Until persisted | Full terminal audit envelopes; deleted only after PostgreSQL persistence succeeds. |

---

## 2. Message Schemas

### A. Job Dispatch Message (`coldharbor:jobs`)
Placed onto Redis Stream by Spring Boot:

```json
{
  "compartmentId": "cpt_a1b2c3d4",
  "context": "INNIE",
  "ownerId": "usr_9918",
  "taskType": "DATA_REDUCTION",
  "payload": {
    "batchSize": 500,
    "inputValues": [10, 25, 42, 99]
  },
  "createdAt": "2026-08-18T10:00:00.000Z"
}
```

### B. Event Notification Message (`coldharbor:events`)
Published to Redis Pub/Sub by Go Workers and Spring Boot:

```json
{
  "eventId": "evt_991203",
  "compartmentId": "cpt_a1b2c3d4",
  "workerId": "worker-go-01",
  "fromState": "RUNNING",
  "toState": "CHECKPOINT",
  "checkpointPct": 50,
  "timestamp": "2026-08-18T10:00:05.120Z",
  "details": "Checkpoint 50% written to scratchpad"
}
```

### C. Dead Drop Payload (`archive:{compartmentId}`)
Sealed JSON stored in Redis string key with TTL:

```json
{
  "compartmentId": "cpt_a1b2c3d4",
  "ownerId": "usr_9918",
  "context": "INNIE",
  "taskType": "DATA_REDUCTION",
  "output": {
    "processedCount": 500,
    "reducedSum": 49201,
    "status": "SUCCESS"
  },
  "checksum": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "archivedAt": "2026-08-18T10:00:10.000Z",
  "ttlSeconds": 3600
}
```

### D. Worker Heartbeat Payload (`worker:{workerId}:heartbeat`)
Liveness indicator stored with 30s TTL:

```json
{
  "workerId": "worker-go-01",
  "status": "BUSY",
  "activeCompartmentId": "cpt_a1b2c3d4",
  "timestamp": "2026-08-18T10:00:05.000Z"
}
```

---

## 3. Finite State Machine Transitions

| From State | To State | Trigger | Actor | Action Taken |
|---|---|---|---|---|
| `CREATED` | `QUEUED` | Job dispatched to Redis Stream | Spring Boot | `XADD coldharbor:jobs` |
| `QUEUED` | `RUNNING` | Worker claims job via consumer group | Go Worker | `XREADGROUP`, initializes scratchpad hash |
| `RUNNING` | `CHECKPOINT` | Intermediate progress milestone | Go Worker | `HSET compartment:{id}:mem`, emits event |
| `CHECKPOINT` | `RUNNING` | Checkpoint acknowledged | Go Worker | Resumes execution |
| `RUNNING` / `CHECKPOINT` | `COMPLETED` | Task logic finished successfully | Go Worker | Computes output SHA-256 |
| `COMPLETED` | `ARCHIVED` | Dead Drop sealed | Go Worker | `SET archive:{id} ... EX {ttl}` |
| `ARCHIVED` | `PURGED` | Ephemeral cleanup | Go Worker & Spring | Go: `DEL compartment:{id}:mem`, Spring: Saves record to Postgres |
| `RUNNING` / `CHECKPOINT` | `FAILED` | Unrecoverable error | Go Worker | Logs error, writes failure audit, initiates `PURGED` |

---

## 4. Security & Context Isolation Rules

1. **Context Clearances**:
   - `INNIE`: Internal confidential workflows. Restricted strictly to authorized internal sessions.
   - `OUTIE`: Public/external workflows. Cannot read or reference `INNIE` compartments.
   - `SYSTEM`: Automated orchestrators and background wellness monitors.
   - `ADMIN`: Full administrative audit and quarantine clearance.
2. **Zero-Leak Guarantee**:
   - The key `compartment:{id}:mem` must NEVER exist after a compartment reaches `PURGED` state.
   - Workers must call `DEL compartment:{id}:mem` in both success and failure exit branches.

# ColdHarbor Test Infrastructure & Verification Specification (TEST_INFRA.md)

## 1. Executive Summary & Verification Methodology

ColdHarbor is a distributed, compartmentalized task execution platform with zero-trust context isolation, crash resilience, and an immutable audit trail. This document establishes the authoritative **4-Tier End-to-End (E2E) Test Architecture** governing the verification of the platform.

### Core Verification Principles
1. **Opaque-Box & Requirement-Driven**: Tests interact strictly via public interfaces (REST APIs, Redis Streams, Redis Hashes, Pub/Sub channels, WebSocket connections, and PostgreSQL tables). Tests never depend on private runtime internals.
2. **Deterministic Output Derivation**: Every test case derives its expected observable outputs from authoritative specifications (`ORIGINAL_REQUEST.md`, `PROJECT.md`, `docs/contracts.md`, and `docker/init-db.sql`). Facade or always-passing tests are strictly prohibited.
3. **Zero-Leak & Zero-Trust Verification**: Scratchpad memory (`compartment:{compartmentId}:mem`) must be proven non-existent (`EXISTS == 0`) across all terminal execution states. Context boundaries (`INNIE` vs `OUTIE`) are validated across all data access paths.
4. **Autonomous Harness Execution**: The entire test suite can be verified via the PowerShell test harness (`scripts/verify_e2e.ps1`), capable of running against live multi-service environments or isolated protocol mock harnesses.

```
+=========================================================================+
|                  Tier 4: Real-World Scenarios (R4)                      |
|      Full multi-component end-to-end integration & failover paths       |
+-------------------------------------------------------------------------+
|                  Tier 3: Pairwise Combinations                          |
|         Cross-feature interaction matrix between subsystems             |
+-------------------------------------------------------------------------+
|                  Tier 2: Boundary & Corner Cases                        |
|        Stress, empty states, malformed inputs, limit enforcement        |
+-------------------------------------------------------------------------+
|                  Tier 1: Feature Coverage (>=5 per feature)             |
|        Deterministic validation of all 23 platform capabilities         |
+=========================================================================+
```

## 2. Authoritative Output Derivation Sources

| Specification Document | Authoritative Scope & Contracts Extracted |
|---|---|
| `ORIGINAL_REQUEST.md` | Core functional requirements Section R1-R4; 6 mandatory integration scenarios; zero-leak acceptance criteria; SHA-256 seal invariants. |
| `PROJECT.md` | Feature inventory (23 discrete features); subsystem responsibilities; code layout; milestones M1-M4. |
| `docs/contracts.md` | Protocol keyspaces; Redis Stream message schemas; Pub/Sub event envelope; Dead Drop JSON format; FSM transition matrix; security clearances (`INNIE`, `OUTIE`, `SYSTEM`, `ADMIN`). |
| `docker/init-db.sql` | PostgreSQL schema for `audit_records`; UUID generation; enum check constraints (`context`, `final_state`); column index configurations. |

## 3. Feature Inventory & Subsystem Mapping

| Feature ID | Feature Name | Target Subsystem | Milestone | Primary Interface | Tier 1 Cases | Tier 2 Cases |
|---|---|---|---|---|---|---|
| **F01** | Stream Consumer Group | Worker Engine | M1 | `coldharbor:jobs` | 5 | 5 |
| **F02** | Dead-Letter Queue (DLQ) | Worker Engine | M1 | `coldharbor:jobs:dlq` | 5 | 5 |
| **F03** | Finite State Machine (FSM) | Worker Engine | M1 | `State Transition Matrix` | 5 | 5 |
| **F04** | Ephemeral Scratchpad Memory | Worker Engine | M1 | `compartment:{id}:mem` | 5 | 5 |
| **F05** | Atomic Scratchpad Purge | Worker Engine | M1 | `DEL compartment:{id}:mem` | 5 | 5 |
| **F06** | Multi-Step Checkpointing | Worker Engine | M1 | `Checkpoints 25%, 50%, 75%` | 5 | 5 |
| **F07** | Crash Recovery Logic | Worker Engine | M1 | `XAUTOCLAIM / PEL Claim` | 5 | 5 |
| **F08** | Dead Drop Archive Sealing | Worker Engine | M1 | `archive:{id} + SHA-256` | 5 | 5 |
| **F09** | Worker Heartbeat Emitter | Worker Engine | M1 | `worker:{id}:heartbeat` | 5 | 5 |
| **F10** | Event Publisher | Worker Engine | M1 | `coldharbor:events Pub/Sub` | 5 | 5 |
| **F11** | Context Clearance Proxy | Control Plane | M2 | `X-Context INNIE vs OUTIE` | 5 | 5 |
| **F12** | Compartment REST API | Control Plane | M2 | `/api/v1/compartments/**` | 5 | 5 |
| **F13** | Redis Stream Producer | Control Plane | M2 | `XADD coldharbor:jobs` | 5 | 5 |
| **F14** | PostgreSQL Audit Service | Control Plane | M2 | `audit_records table` | 5 | 5 |
| **F15** | WebSocket Event Gateway | Control Plane | M2 | `/ws/events endpoint` | 5 | 5 |
| **F16** | Worker Wellness Monitor | Control Plane | M2 | `worker:*:heartbeat scanner` | 5 | 5 |
| **F17** | Worker Telemetry UI | Dashboard | M3 | `WorkerTelemetry component` | 5 | 5 |
| **F18** | Compartment Lifecycle Tracker | Dashboard | M3 | `LifecycleTracker component` | 5 | 5 |
| **F19** | Live WebSocket Event Feed | Dashboard | M3 | `EventFeed component` | 5 | 5 |
| **F20** | Dead Drop Inspector UI | Dashboard | M3 | `DeadDropInspector component` | 5 | 5 |
| **F21** | Historical Audit Browser | Dashboard | M3 | `AuditBrowser component` | 5 | 5 |
| **F22** | Local Infra Orchestration | Infrastructure | M4 | `docker-compose.yml Redis & PG` | 5 | 5 |
| **F23** | E2E Verification Suite | Quality Assurance | M4 | `scripts/verify_e2e.ps1` | 5 | 5 |

---

## 4. Tier 1: Feature Coverage Suite (115 Test Cases)

Tier 1 provides deterministic functional validation of all 23 platform capabilities, verifying nominal happy paths, protocol adherence, and contract conformity.

### Feature F01: Stream Consumer Group (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `coldharbor:jobs`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F01-01` | **Consumer Group Creation** | Execute XGROUP CREATE coldharbor:jobs worker-group $ MKSTREAM | Redis creates group or returns BUSYGROUP without error | `docs/contracts.md Â§1` |
| `TC-T1-F01-02` | **Disjoint Stream Partitioning** | Multiple workers execute XREADGROUP GROUP worker-group w1/w2 COUNT 1 | Each worker receives disjoint task IDs from the stream | `docs/contracts.md Â§1.1` |
| `TC-T1-F01-03` | **PEL Delivery Tracking** | Worker reads message without acknowledging; query XPENDING coldharbor:jobs worker-group | Message appears in PEL with consumer ID and delivery count 1 | `ORIGINAL_REQUEST.md Â§R1` |
| `TC-T1-F01-04` | **Terminal State Acknowledgment** | Task completes successfully to PURGED state; worker executes XACK | XACK returns 1 and message is evicted from PEL | `docs/contracts.md Â§1.1` |
| `TC-T1-F01-05` | **Abandoned Message Claim** | Simulate worker disconnect; standby worker issues XAUTOCLAIM coldharbor:jobs worker-group w2 1000 0-0 | Unacknowledged message transferred to consumer w2 | `PROJECT.md Â§Milestones (M1)` |

### Feature F02: Dead-Letter Queue (DLQ) (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `coldharbor:jobs:dlq`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F02-01` | **Poison Pill Routing to DLQ** | Submit message with corrupt unparseable JSON payload | Message acknowledged on coldharbor:jobs and forwarded to coldharbor:jobs:dlq | `ORIGINAL_REQUEST.md Â§R1` |
| `TC-T1-F02-02` | **Max Retries Exceeded DLQ Transfer** | Worker repeatedly fails task until delivery count exceeds maxRetries (3) | Worker executes XADD coldharbor:jobs:dlq and XACK on primary stream | `docs/contracts.md Â§1.1` |
| `TC-T1-F02-03` | **DLQ Metadata Enrichment** | Inspect DLQ stream entry fields | Entry contains errorReason, failedAt timestamp, and original payload | `docs/contracts.md Â§1.1` |
| `TC-T1-F02-04` | **DLQ Failure Audit Insertion** | Inspect PostgreSQL audit_records after DLQ transition | Record persisted with final_state=FAILED and error metadata | `ORIGINAL_REQUEST.md Â§R2` |
| `TC-T1-F02-05` | **DLQ Event Notification** | Listen to coldharbor:events during DLQ transfer | Event received with currentState=FAILED and details specifying DLQ routing | `docs/contracts.md Â§2.B` |

### Feature F03: Finite State Machine (FSM) (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `State Transition Matrix`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F03-01` | **Nominal Full FSM Progression** | Execute sequential transitions CREATED -> QUEUED -> RUNNING -> COMPLETED -> ARCHIVED -> PURGED | All transitions succeed and ValidateTransition returns nil | `docs/contracts.md Â§3` |
| `TC-T1-F03-02` | **Checkpoint Iteration Loop** | Execute transitions RUNNING -> CHECKPOINT -> RUNNING -> CHECKPOINT -> COMPLETED | All intermediate transitions validate without error | `docs/contracts.md Â§3` |
| `TC-T1-F03-03` | **Runtime Failure Progression** | Execute transitions RUNNING -> FAILED -> PURGED | Terminal failure path successfully validated | `docs/contracts.md Â§3` |
| `TC-T1-F03-04` | **Checkpoint Failure Progression** | Execute transitions CHECKPOINT -> FAILED -> PURGED | Post-checkpoint failure path successfully validated | `docs/contracts.md Â§3` |
| `TC-T1-F03-05` | **Terminal Immutability** | Attempt ValidateTransition(PURGED, RUNNING) and ValidateTransition(PURGED, COMPLETED) | Both return ErrTerminalState error rejecting modification | `docs/contracts.md Â§3` |

### Feature F04: Ephemeral Scratchpad Memory (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `compartment:{id}:mem`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F04-01` | **Scratchpad Hash Initialization** | Worker transitions to RUNNING and initializes compartment:{id}:mem | EXISTS compartment:{id}:mem returns 1 as Redis Hash | `docs/contracts.md Â§1.2` |
| `TC-T1-F04-02` | **Intermediate Step Persistence** | Worker executes HSET compartment:{id}:mem current_step 1 progress_pct 25 | HGETALL returns current_step=1 and progress_pct=25 | `docs/contracts.md Â§1.2` |
| `TC-T1-F04-03` | **State Accumulation Across Steps** | Worker updates intermediate_result JSON across steps 1, 2, 3 | intermediate_result retains cumulative computations correctly | `ORIGINAL_REQUEST.md Â§R1` |
| `TC-T1-F04-04` | **Checkpoint Timestamp Tracking** | Worker updates last_checkpoint_at on checkpoint milestone | Field contains valid ISO-8601 UTC timestamp string | `docs/contracts.md Â§1.2` |
| `TC-T1-F04-05` | **Multi-Compartment Isolation** | Simultaneously execute two tasks with distinct IDs A and B | compartment:A:mem and compartment:B:mem maintain independent states | `PROJECT.md Â§Architecture` |

### Feature F05: Atomic Scratchpad Purge (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `DEL compartment:{id}:mem`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F05-01` | **Successful Completion Atomic Purge** | Task enters PURGED state following successful archive | Worker issues DEL compartment:{id}:mem; EXISTS returns 0 | `ORIGINAL_REQUEST.md Â§R1, docs/contracts.md Â§1.2` |
| `TC-T1-F05-02` | **Fatal Failure Atomic Purge** | Task fails and enters PURGED state from FAILED | Deferred cleanup issues DEL compartment:{id}:mem; EXISTS returns 0 | `docs/contracts.md Â§4.2` |
| `TC-T1-F05-03` | **Zero Memory Leak Verification** | Execute 10 complete tasks to PURGED state; scan KEYS compartment:*:mem | Keyspace scan returns zero residual scratchpad keys | `ORIGINAL_REQUEST.md Â§R4 (Scenario 5)` |
| `TC-T1-F05-04` | **Successor Cleanup on Recovery** | Successor worker completes recovered task; issues purge | Scratchpad deleted before stream XACK acknowledged | `PROJECT.md Â§Milestones (M1)` |
| `TC-T1-F05-05` | **Idempotent Purge Invocation** | Worker issues DEL on already non-existent scratchpad key | DEL returns 0 without raising runtime exception or halt | `docs/contracts.md Â§1.2` |

### Feature F06: Multi-Step Checkpointing (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `Checkpoints 25%, 50%, 75%`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F06-01` | **Checkpoint Milestone 25%** | Task executes first milestone step | Scratchpad has progress_pct=25, current_step=1; event published | `ORIGINAL_REQUEST.md Â§R1` |
| `TC-T1-F06-02` | **Checkpoint Milestone 50%** | Task executes second milestone step | Scratchpad has progress_pct=50, current_step=2; event published | `ORIGINAL_REQUEST.md Â§R1` |
| `TC-T1-F06-03` | **Checkpoint Milestone 75%** | Task executes third milestone step | Scratchpad has progress_pct=75, current_step=3; event published | `ORIGINAL_REQUEST.md Â§R1` |
| `TC-T1-F06-04` | **FSM State Synchronization** | Inspect compartment status during checkpoint write | State alternates RUNNING -> CHECKPOINT -> RUNNING smoothly | `docs/contracts.md Â§3` |
| `TC-T1-F06-05` | **Intermediate Serialization Fidelity** | Extract intermediate_result JSON string at 50% checkpoint | JSON parses cleanly and matches domain schema | `docs/contracts.md Â§1.2` |

### Feature F07: Crash Recovery Logic (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `XAUTOCLAIM / PEL Claim`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F07-01` | **Worker Crash Emulation & Claim** | Worker killed mid-execution at step 2; standby issues XAUTOCLAIM | Standby claims message ID and re-enters RUNNING | `ORIGINAL_REQUEST.md Â§R4 (Scenario 3)` |
| `TC-T1-F07-02` | **Checkpoint Resumption Offset** | Successor reads scratchpad with current_step=2 | Execution skips steps 1 and 2, resuming calculation at step 3 | `ORIGINAL_REQUEST.md Â§R1` |
| `TC-T1-F07-03` | **Completed Step Idempotency** | Verify side effects of steps 1 and 2 during resumption | No duplicate execution of steps 1 and 2 occurs | `ORIGINAL_REQUEST.md Â§Acceptance Criteria` |
| `TC-T1-F07-04` | **Pre-Checkpoint Crash Recovery** | Worker killed before step 1 (progress 0%); successor claims | Successor initializes fresh scratchpad and starts from step 1 | `PROJECT.md Â§Milestones (M1)` |
| `TC-T1-F07-05` | **Recovery Retry Budget** | Successor increments delivery attempt counter upon reclaim | Attempt counter verified in stream metadata | `docs/contracts.md Â§1.1` |

### Feature F08: Dead Drop Archive Sealing (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `archive:{id} + SHA-256`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F08-01` | **Dead Drop Key Sealing** | Task finishes COMPLETED state; worker seals archive:{id} | Redis key archive:{id} exists with JSON payload | `ORIGINAL_REQUEST.md Â§R1, docs/contracts.md Â§1.3` |
| `TC-T1-F08-02` | **SHA-256 Seal Validation** | Compute sha256 of canonical JSON output and compare to archive checksum | Checksum in archive:{id} matches computed SHA-256 hexadecimal string exactly | `ORIGINAL_REQUEST.md Â§R1` |
| `TC-T1-F08-03` | **Dead Drop Configurable TTL** | Check TTL of archive:{id} with default TTL 3600s | TTL returns value between 3500 and 3600 seconds | `docs/contracts.md Â§1.3` |
| `TC-T1-F08-04` | **Dead Drop Payload Completeness** | Inspect JSON structure of archive:{id} | Contains compartmentId, ownerId, context, taskType, output, checksum, archivedAt | `docs/contracts.md Â§2.C` |
| `TC-T1-F08-05` | **Dead Drop Auto-Destruction** | Set short TTL (2s) on archive:{id} and wait 3s | EXISTS archive:{id} returns 0 after TTL expiry | `ORIGINAL_REQUEST.md Â§R4 (Scenario 4)` |

### Feature F09: Worker Heartbeat Emitter (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `worker:{id}:heartbeat`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F09-01` | **Worker Heartbeat Publication** | Worker starts; inspect worker:{workerId}:heartbeat | Key exists with TTL between 20 and 30 seconds | `docs/contracts.md Â§1.4` |
| `TC-T1-F09-02` | **Heartbeat Periodic Refresh** | Observe heartbeat key TTL over 25 seconds | TTL refreshed periodically every 10 seconds, never dropping to 0 | `ORIGINAL_REQUEST.md Â§R1` |
| `TC-T1-F09-03` | **Heartbeat Payload Schema** | Parse JSON value of worker:{workerId}:heartbeat | Valid JSON with workerId, status, and ISO-8601 timestamp | `docs/contracts.md Â§2.D` |
| `TC-T1-F09-04` | **Active Compartment Reporting** | Worker begins processing task; inspect heartbeat | Heartbeat JSON reflects status=BUSY and activeCompartmentId populated | `docs/contracts.md Â§2.D` |
| `TC-T1-F09-05` | **Heartbeat Graceful Expiry on Crash** | Terminate worker process abruptly; wait 35 seconds | worker:{workerId}:heartbeat auto-expires and returns EXISTS=0 | `ORIGINAL_REQUEST.md Â§R2` |

### Feature F10: Event Publisher (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `coldharbor:events Pub/Sub`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F10-01` | **Pub/Sub Transition Broadcasting** | Subscribe to coldharbor:events during job lifecycle | Subscriber receives event stream for every state transition | `ORIGINAL_REQUEST.md Â§R1, docs/contracts.md Â§1.5` |
| `TC-T1-F10-02` | **Event Envelope Validation** | Validate JSON schema of received events | Includes eventId, compartmentId, workerId, fromState, toState, timestamp | `docs/contracts.md Â§2.B` |
| `TC-T1-F10-03` | **Checkpoint Progress Metadata** | Inspect event where toState=CHECKPOINT | Contains checkpointPct with integer value 25, 50, or 75 | `docs/contracts.md Â§2.B` |
| `TC-T1-F10-04` | **Failure Event Details** | Inspect event where toState=FAILED | details field contains descriptive failure message | `docs/contracts.md Â§2.B` |
| `TC-T1-F10-05` | **Sub-Second Broadcast Latency** | Measure delta between worker state transition and event receipt | Delta is less than 50ms over local Redis connection | `ORIGINAL_REQUEST.md Â§Acceptance Criteria` |

### Feature F11: Context Clearance Proxy (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `X-Context INNIE vs OUTIE`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F11-01` | **INNIE Context Authorization** | POST /api/v1/compartments with header X-Context: INNIE | HTTP 201 Created; compartment created in INNIE namespace | `docs/contracts.md Â§4.1` |
| `TC-T1-F11-02` | **OUTIE Context Authorization** | POST /api/v1/compartments with header X-Context: OUTIE | HTTP 201 Created; compartment created in OUTIE namespace | `docs/contracts.md Â§4.1` |
| `TC-T1-F11-03` | **Cross-Context Clearance Rejection** | GET /api/v1/compartments/{innieId} with header X-Context: OUTIE | HTTP 403 Forbidden; access to internal compartment denied | `docs/contracts.md Â§4.1` |
| `TC-T1-F11-04` | **Missing Clearance Header** | POST /api/v1/compartments without X-Context header | HTTP 400 Bad Request indicating missing context clearance header | `docs/contracts.md Â§4.1` |
| `TC-T1-F11-05` | **Privileged Context Clearance** | GET /api/v1/compartments/{innieId} with header X-Context: ADMIN | HTTP 200 OK; administrative clearance granted | `docs/contracts.md Â§4.1` |

### Feature F12: Compartment REST API (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `/api/v1/compartments/**`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F12-01` | **Compartment Submission Endpoint** | POST /api/v1/compartments with valid task payload | HTTP 201 Created with JSON body containing compartmentId and status=QUEUED | `docs/contracts.md Â§3, Â§5` |
| `TC-T1-F12-02` | **Compartment Status Query Endpoint** | GET /api/v1/compartments/{id} with authorized context | HTTP 200 OK with state, progressPct, currentStep, and timestamps | `PROJECT.md Â§Interface Contracts (5)` |
| `TC-T1-F12-03` | **Dead Drop Retrieval Endpoint** | GET /api/v1/compartments/{id}/deaddrop for completed task | HTTP 200 OK with output JSON, checksum, and ttlSecondsRemaining | `PROJECT.md Â§Interface Contracts (5)` |
| `TC-T1-F12-04` | **Historical Audit Query Endpoint** | GET /api/v1/audits with filter context=INNIE | HTTP 200 OK with JSON array of matching audit records | `PROJECT.md Â§Interface Contracts (5)` |
| `TC-T1-F12-05` | **Active Worker Discovery Endpoint** | GET /api/v1/workers | HTTP 200 OK with JSON array of active workers and liveness status | `PROJECT.md Â§Interface Contracts (5)` |

### Feature F13: Redis Stream Producer (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `XADD coldharbor:jobs`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F13-01` | **REST Dispatch to Redis Stream** | POST /api/v1/compartments and verify Redis Stream | Control Plane issues XADD coldharbor:jobs with valid JobMessage | `ORIGINAL_REQUEST.md Â§R2` |
| `TC-T1-F13-02` | **Job Message Payload Structure** | Inspect message generated on coldharbor:jobs | Contains compartmentId, ownerId, context, taskType, payload, createdAt | `docs/contracts.md Â§2.A` |
| `TC-T1-F13-03` | **Unique Stream Message ID** | Submit two consecutive jobs via REST API | Redis Stream assigns distinct monotonic message IDs | `docs/contracts.md Â§1.1` |
| `TC-T1-F13-04` | **Default Configuration Injection** | Submit job without explicit timeout or retries | Producer populates default maxRetries=3 and timeoutSeconds=300 | `docs/contracts.md Â§2.A` |
| `TC-T1-F13-05` | **Producer Error Handling** | Simulate Redis disconnection during dispatch | API returns HTTP 503 Service Unavailable with descriptive error message | `PROJECT.md Â§Milestones (M2)` |

### Feature F14: PostgreSQL Audit Service (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `audit_records table`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F14-01` | **Durable Completion Audit Record** | Compartment reaches PURGED state successfully | Record inserted into PostgreSQL audit_records with final_state=PURGED | `ORIGINAL_REQUEST.md Â§R2, Â§R4 (Scenario 6)` |
| `TC-T1-F14-02` | **Audit Checksum Integrity** | Compare checksum in PostgreSQL audit_records to Dead Drop archive | Database checksum matches archive checksum exactly | `ORIGINAL_REQUEST.md Â§Acceptance Criteria` |
| `TC-T1-F14-03` | **Audit Execution Duration** | Verify duration_ms in audit_records | duration_ms equals completed_at minus created_at in milliseconds (>= 0) | `docker/init-db.sql` |
| `TC-T1-F14-04` | **Audit Failure Disposition** | Compartment reaches terminal FAILED state | Record inserted into audit_records with final_state=FAILED and error metadata | `docker/init-db.sql` |
| `TC-T1-F14-05` | **Audit Relational Querying** | Execute SELECT * FROM audit_records WHERE compartment_id = :id | Query returns exactly one row with matching attributes and valid UUID primary key | `docker/init-db.sql` |

### Feature F15: WebSocket Event Gateway (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `/ws/events endpoint`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F15-01` | **WebSocket Gateway Handshake** | Establish WebSocket connection to /ws/events | HTTP 101 Switching Protocols; socket connection established | `ORIGINAL_REQUEST.md Â§R2` |
| `TC-T1-F15-02` | **Pub/Sub to WebSocket Relay** | Publish event to coldharbor:events; observe WebSocket client | Client receives real-time JSON frame with identical event data | `PROJECT.md Â§Milestones (M2)` |
| `TC-T1-F15-03` | **Multi-Client WebSocket Fanout** | Connect 3 WebSocket clients; publish single event | All 3 clients receive identical event message simultaneously | `docs/contracts.md Â§5` |
| `TC-T1-F15-04` | **WebSocket Client Disconnect Cleanliness** | Client terminates WebSocket connection unexpectedly | Gateway logs disconnect and cleans session resources without memory leak | `PROJECT.md Â§Milestones (M2)` |
| `TC-T1-F15-05` | **WebSocket Sub-Second Latency** | Measure elapsed time between event publish and WebSocket frame receipt | Receipt latency is under 100ms | `ORIGINAL_REQUEST.md Â§Acceptance Criteria` |

### Feature F16: Worker Wellness Monitor (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `worker:*:heartbeat scanner`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F16-01` | **Active Worker Fleet Discovery** | Deploy 2 workers pulsing heartbeats; query wellness monitor | Wellness monitor discovers both worker IDs as HEALTHY | `ORIGINAL_REQUEST.md Â§R2` |
| `TC-T1-F16-02` | **Missing Heartbeat Dead Worker Flag** | Stop heartbeat emission from worker-01; wait 30s | Wellness monitor marks worker-01 as DEAD | `ORIGINAL_REQUEST.md Â§R2` |
| `TC-T1-F16-03` | **Dead Worker Restoration** | Resume heartbeat emission for worker-01 | Wellness monitor restores worker-01 status to ACTIVE/HEALTHY | `PROJECT.md Â§Milestones (M2)` |
| `TC-T1-F16-04` | **Wellness REST API Exposure** | GET /api/v1/workers | Returns JSON list of all discovered workers with status, liveness, and active task | `PROJECT.md Â§Interface Contracts (5)` |
| `TC-T1-F16-05` | **Dead Worker Task Recovery Trigger** | Worker crashes while processing compartment; monitor detects death | Monitor or consumer group reclaims orphaned task for execution | `ORIGINAL_REQUEST.md Â§R1, Â§R2` |

### Feature F17: Worker Telemetry UI (Dashboard)
- **Target Milestone**: M3 | **Primary Interface**: `WorkerTelemetry component`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F17-01` | **Worker Telemetry Rendering** | Render WorkerTelemetry component in React UI | Component displays worker cards with ID, status, and liveness badge | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F17-02` | **Active Task Display** | Worker is BUSY processing compartment cpt_123 | Worker card displays active compartment ID cpt_123 with link to tracker | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F17-03` | **Dead Worker Visual Warning** | Worker marked DEAD in API payload | Card transitions to red/warning border with Inactive badge | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F17-04` | **Telemetry Auto-Refresh** | Simulate status change in REST endpoint; wait poll interval | UI updates worker card status without requiring full page reload | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F17-05` | **Empty Fleet Handling** | API returns empty worker array [] | Component displays No active workers detected message gracefully | `PROJECT.md Â§Milestones (M3)` |

### Feature F18: Compartment Lifecycle Tracker (Dashboard)
- **Target Milestone**: M3 | **Primary Interface**: `LifecycleTracker component`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F18-01` | **Lifecycle Tracker Node Rendering** | Render LifecycleTracker component for active compartment | Displays all 7 lifecycle states (CREATED, QUEUED, RUNNING, CHECKPOINT, COMPLETED, ARCHIVED, PURGED) | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F18-02` | **Active State Highlighting** | Compartment is in RUNNING state | RUNNING node highlighted with active animation / glowing border | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F18-03` | **Checkpoint Progress Fill** | Compartment reaches 50% milestone | Progress bar updates to 50% width and displays Step 2 of 3 | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F18-04` | **Terminal Purged Confirmation** | Compartment reaches PURGED state | Purged node highlighted green with Ephemeral Memory Cleared badge | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F18-05` | **Failed State Red Direction** | Compartment encounters fatal error | FAILED node highlighted red and error message banner displayed | `ORIGINAL_REQUEST.md Â§R3` |

### Feature F19: Live WebSocket Event Feed (Dashboard)
- **Target Milestone**: M3 | **Primary Interface**: `EventFeed component`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F19-01` | **WebSocket Event Stream Attachment** | Mount EventFeed component with active WebSocket connection | Displays Live indicator and connects to /ws/events | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F19-02` | **Chronological Event Ingestion** | Emit sequence of 5 events across lifecycle | Feed displays all 5 event items in reverse-chronological order | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F19-03` | **Event Item Schema Display** | Inspect rendered event row in feed | Displays timestamp, compartment ID, worker ID, fromState -> toState | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F19-04` | **Feed Pause/Resume Controls** | Click Pause button in EventFeed | Stream updates freeze; clicking Resume displays all buffered events | `PROJECT.md Â§Milestones (M3)` |
| `TC-T1-F19-05` | **Socket Disconnect Indicator** | Simulate WebSocket server disconnect | Live indicator switches to Reconnecting... (amber) | `ORIGINAL_REQUEST.md Â§Acceptance Criteria` |

### Feature F20: Dead Drop Inspector UI (Dashboard)
- **Target Milestone**: M3 | **Primary Interface**: `DeadDropInspector component`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F20-01` | **Dead Drop Query by ID** | Enter compartment ID into DeadDropInspector input and submit | Component fetches /api/v1/compartments/{id}/deaddrop and renders payload | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F20-02` | **SHA-256 Seal Validation Display** | Sealed output loaded in inspector | Displays SHA-256 fingerprint with Verified Seal green checkmark badge | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F20-03` | **TTL Expiry Countdown** | Observe TTL display for retrieved dead drop | Displays active countdown timer showing minutes/seconds remaining | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F20-04` | **Formatted JSON Payload Inspector** | Inspect structured JSON result in viewer | Component renders formatted JSON with syntax highlighting and copy button | `PROJECT.md Â§Milestones (M3)` |
| `TC-T1-F20-05` | **Expired Dead Drop Message** | Query compartment whose TTL has expired (404 response) | Displays Dead Drop Expired or Purged info notice | `ORIGINAL_REQUEST.md Â§R3` |

### Feature F21: Historical Audit Browser (Dashboard)
- **Target Milestone**: M3 | **Primary Interface**: `AuditBrowser component`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F21-01` | **Audit History Table Rendering** | Mount AuditBrowser component | Fetches /api/v1/audits and displays rows with UUID, Compartment, Context, Duration | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F21-02` | **Context Clearance Filtering** | Select INNIE from context filter dropdown | Table filters to show only records matching context INNIE | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F21-03` | **Metadata Modal Inspection** | Click View Metadata on audit row | Modal opens displaying JSONB metadata with worker ID and execution telemetry | `ORIGINAL_REQUEST.md Â§R3` |
| `TC-T1-F21-04` | **Checksum Cross-Verification** | Verify checksum displayed in audit browser row | Matches the Dead Drop SHA-256 fingerprint for the compartment | `ORIGINAL_REQUEST.md Â§Acceptance Criteria` |
| `TC-T1-F21-05` | **Audit History Pagination** | Click Next page in audit browser | Loads next page of historical records with updated offset parameter | `PROJECT.md Â§Milestones (M3)` |

### Feature F22: Local Infra Orchestration (Infrastructure)
- **Target Milestone**: M4 | **Primary Interface**: `docker-compose.yml Redis & PG`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F22-01` | **Docker Compose Service Initialization** | Execute docker-compose up -d in docker/ directory | Both coldharbor-redis and coldharbor-postgres containers start cleanly | `ORIGINAL_REQUEST.md Â§R4` |
| `TC-T1-F22-02` | **Redis Container Healthcheck** | Query health status of coldharbor-redis | Container reports healthy via redis-cli ping | `docker/docker-compose.yml` |
| `TC-T1-F22-03` | **PostgreSQL Container Healthcheck** | Query health status of coldharbor-postgres | Container reports healthy via pg_isready | `docker/docker-compose.yml` |
| `TC-T1-F22-04` | **Database Schema Auto-Initialization** | Inspect tables in PostgreSQL coldharbor database | Table audit_records exists with all columns and indices from init-db.sql | `docker/init-db.sql` |
| `TC-T1-F22-05` | **Docker Port Expositions** | Verify host port bindings | Port 6379 bound to Redis, port 5432 bound to PostgreSQL | `docker/docker-compose.yml` |

### Feature F23: E2E Verification Suite (Quality Assurance)
- **Target Milestone**: M4 | **Primary Interface**: `scripts/verify_e2e.ps1`

| Test ID | Test Name | Input / Preconditions | Expected Observable Output | Authoritative Source |
|---|---|---|---|---|
| `TC-T1-F23-01` | **Verification Script Execution** | Execute powershell -File scripts/verify_e2e.ps1 | Harness executes all scenarios sequentially and exits with code 0 | `ORIGINAL_REQUEST.md Â§R4` |
| `TC-T1-F23-02` | **Scenario 1 Verification** | Execute Scenario 1 (REST API Job Submission) | Verifies job dispatch, compartment creation, and stream presence | `ORIGINAL_REQUEST.md Â§R4 (Scenario 1)` |
| `TC-T1-F23-03` | **Scenario 2 Verification** | Execute Scenario 2 (Worker Claim & Checkpoints) | Verifies consumer group claim, scratchpad hash write, and 3 checkpoints | `ORIGINAL_REQUEST.md Â§R4 (Scenario 2)` |
| `TC-T1-F23-04` | **Scenario 3 Verification** | Execute Scenario 3 (Worker Crash Simulation & Recovery) | Verifies crash recovery from exact intermediate checkpoint | `ORIGINAL_REQUEST.md Â§R4 (Scenario 3)` |
| `TC-T1-F23-05` | **Scenarios 4, 5, 6 Verification** | Execute Scenarios 4, 5, 6 (Dead Drop, Zero-Leak, Postgres Audit) | Verifies SHA-256 seal, scratchpad DEL purge, and audit persistence | `ORIGINAL_REQUEST.md Â§R4 (Scenarios 4-6)` |

---

## 5. Tier 2: Boundary & Corner Cases Suite (115 Test Cases)

Tier 2 subjects all 23 features to boundary conditions, corner cases, malformed payloads, resource exhaustion, and fault recovery invariants.

### Feature F01 Boundary & Corner: Stream Consumer Group (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `coldharbor:jobs`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F01-01` | **Empty Stream Polling Timeout** | Execute XREADGROUP BLOCK 2000 on empty coldharbor:jobs | Returns nil cleanly after 2000ms without thread starvation or busy-wait | `docs/contracts.md Section 1.1` |
| `TC-T2-F01-02` | **Idempotent Consumer Group Creation** | Execute XGROUP CREATE coldharbor:jobs worker-group when group already exists | Handles BUSYGROUP error gracefully without failing service boot | `services/worker-engine` |
| `TC-T2-F01-03` | **Malformed Stream Field Handling** | Inject stream entry missing taskType or compartmentId | Worker validates fields, logs parse failure, and avoids panic | `services/worker-engine/internal/model/job.go` |
| `TC-T2-F01-04` | **High Volume Stream Burst** | Produce 1000 messages onto stream rapidly; workers consume concurrently | All 1000 messages processed without dropped entries or deadlock | `ORIGINAL_REQUEST.md Section R1` |
| `TC-T2-F01-05` | **Invalid XACK Message ID** | Execute XACK coldharbor:jobs worker-group non-existent-id-0 | Returns 0 acknowledging no message without throwing exception | `docs/contracts.md Section 1.1` |

### Feature F02 Boundary & Corner: Dead-Letter Queue (DLQ) (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `coldharbor:jobs:dlq`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F02-01` | **Zero Max Retries DLQ Routing** | Submit task configured with maxRetries=0 that encounters error | Immediately routed to coldharbor:jobs:dlq on initial failure | `ORIGINAL_REQUEST.md Section R1` |
| `TC-T2-F02-02` | **Large Payload Poison Pill** | Submit 5MB malformed payload that worker cannot parse | Successfully stored in DLQ preserving full error context | `docs/contracts.md Section 1.1` |
| `TC-T2-F02-03` | **Concurrent DLQ Routing Race** | Two workers attempt to DLQ the same timed-out message simultaneously | Exactly one worker successfully executes XADD DLQ and XACK | `PROJECT.md Section Milestones (M1)` |
| `TC-T2-F02-04` | **Raw Binary Corruption in DLQ** | Stream entry corrupted with non-UTF-8 binary bytes | DLQ consumer logs warning and retains raw bytes in error field | `docs/contracts.md Section 1.1` |
| `TC-T2-F02-05` | **DLQ Stream Retention & No-Loss** | Push 50 poison pill jobs to DLQ | All 50 jobs retainable and inspectable for diagnostic replay | `ORIGINAL_REQUEST.md Section R1` |

### Feature F03 Boundary & Corner: Finite State Machine (FSM) (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `State Transition Matrix`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F03-01` | **Illegal Forward Jump Rejection** | Attempt ValidateTransition(CREATED, COMPLETED) | Returns error: illegal state transition CREATED -> COMPLETED | `docs/contracts.md Section 3` |
| `TC-T2-F03-02` | **Illegal Backward Transition Rejection** | Attempt ValidateTransition(COMPLETED, RUNNING) | Returns error: illegal state transition COMPLETED -> RUNNING | `docs/contracts.md Section 3` |
| `TC-T2-F03-03` | **Terminal State Resurrection Rejection** | Attempt ValidateTransition(PURGED, RUNNING) | Returns ErrTerminalState error rejecting modification | `docs/contracts.md Section 3` |
| `TC-T2-F03-04` | **Self-Transition Idempotency Rejection** | Attempt ValidateTransition(RUNNING, RUNNING) | Returns illegal self-transition error to prevent state stagnation | `services/worker-engine/internal/model/state.go` |
| `TC-T2-F03-05` | **Invalid State String Rejection** | Pass arbitrary string FOOBAR to state parser | Returns ErrInvalidState error rejecting unknown state | `services/worker-engine/internal/model/state.go` |

### Feature F04 Boundary & Corner: Ephemeral Scratchpad Memory (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `compartment:{id}:mem`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F04-01` | **Maximum Scratchpad Payload Size** | Write 10MB intermediate aggregate into intermediate_result field | HSET succeeds and HGET retrieves intact payload | `ORIGINAL_REQUEST.md Section R1` |
| `TC-T2-F04-02` | **Unicode and Binary Escaping in Scratchpad** | Store multi-byte unicode and escaped JSON in intermediate_result | Data retrieved with byte-for-byte fidelity | `PROJECT.md Section Interface Contracts` |
| `TC-T2-F04-03` | **Missing Scratchpad Query** | Query HGETALL on non-existent compartment ID | Returns empty hash map without throwing nil pointer exception | `docs/contracts.md Section 1.2` |
| `TC-T2-F04-04` | **Concurrent Hash Field Updates** | Simultaneously update progress_pct and intermediate_result from separate goroutines | Atomic hash commands execute without race corruption | `services/worker-engine` |
| `TC-T2-F04-05` | **Scratchpad Write Under Low Memory** | Simulate Redis near maxmemory limit during scratchpad write | Worker catches OOM error and safely routes task to FAILED | `ORIGINAL_REQUEST.md Section R1` |

### Feature F05 Boundary & Corner: Atomic Scratchpad Purge (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `DEL compartment:{id}:mem`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F05-01` | **Double Purge Idempotency** | Execute DEL compartment:{id}:mem twice consecutively | First call returns 1, second call returns 0; no exception thrown | `docs/contracts.md Section 1.2` |
| `TC-T2-F05-02` | **Purge of Never-Created Scratchpad** | Execute purge routine on job that failed in QUEUED before worker claim | DEL returns 0 cleanly and purge proceeds to audit persistence | `docs/contracts.md Section 4.2` |
| `TC-T2-F05-03` | **Concurrent High-Rate Purges** | Issue 100 simultaneous DEL commands for completed compartments | All 100 keys purged in <100ms with zero Redis lockup | `ORIGINAL_REQUEST.md Section R4 (Scenario 5)` |
| `TC-T2-F05-04` | **Transient Redis Disconnection on Purge** | Simulate network drop during DEL command | Worker retries DEL with exponential backoff until confirmed gone | `docs/contracts.md Section 4.2` |
| `TC-T2-F05-05` | **Prefix Protection During Purge** | Verify DEL command targets exact key compartment:{id}:mem only | Keys for other compartments like compartment:{other}:mem are untouched | `PROJECT.md Section Architecture` |

### Feature F06 Boundary & Corner: Multi-Step Checkpointing (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `Checkpoints 25%, 50%, 75%`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F06-01` | **Rapid Succession Checkpoints** | Emit checkpoints at 25%, 50%, 75% in sub-millisecond succession | All checkpoints written in order and events published monotonically | `docs/contracts.md Section 2.B` |
| `TC-T2-F06-02` | **Non-Standard Progress Percentages** | Emit checkpoint with progress_pct=33 or 66 | FSM accepts dynamic integer progress percentages safely | `docs/contracts.md Section 2.B` |
| `TC-T2-F06-03` | **Redis Write Timeout on Checkpoint** | Simulate Redis timeout during intermediate HSET | Worker logs checkpoint error and retries or safely triggers failover | `ORIGINAL_REQUEST.md Section R1` |
| `TC-T2-F06-04` | **Monotonic Progress Enforcement** | Attempt checkpoint write with progress_pct lower than current | Worker rejects backward progress reporting | `services/worker-engine` |
| `TC-T2-F06-05` | **Zero Intermediate Data Checkpoint** | Emit checkpoint with empty intermediate result object {} | HSET succeeds with valid empty JSON string | `docs/contracts.md Section 1.2` |

### Feature F07 Boundary & Corner: Crash Recovery Logic (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `XAUTOCLAIM / PEL Claim`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F07-01` | **Crash During Checkpoint HSET** | Kill worker during exact moment of HSET write to Redis | Successor detects incomplete write and rolls back to prior checkpoint | `ORIGINAL_REQUEST.md Section R1` |
| `TC-T2-F07-02` | **Corrupted Scratchpad JSON Resumption** | Successor finds corrupt non-JSON intermediate_result in scratchpad | Logs corrupted state, falls back to re-executing from step 1 | `PROJECT.md Section Milestones (M1)` |
| `TC-T2-F07-03` | **Exhausted Crash Recovery Budget** | Crash worker on step 3 three consecutive times | Task exceeds maxRetries and is diverted to DLQ on 4th attempt | `ORIGINAL_REQUEST.md Section R1` |
| `TC-T2-F07-04` | **XAUTOCLAIM Race Condition** | Two standby workers run XAUTOCLAIM concurrently on same crashed task | Redis PEL semantics award ownership to exactly one worker | `docs/contracts.md Section 1.1` |
| `TC-T2-F07-05` | **Min-Idle-Time Guardrail** | Execute XAUTOCLAIM with minIdleTime=10000ms on task running for 2000ms | Claim returns empty list, preventing premature seizure of running job | `docs/contracts.md Section 1.1` |

### Feature F08 Boundary & Corner: Dead Drop Archive Sealing (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `archive:{id} + SHA-256`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F08-01` | **Unordered JSON Map Key Invariance** | Serialize output payload with keys in different order | Canonical JSON serializer generates identical SHA-256 checksum | `ORIGINAL_REQUEST.md Section Acceptance Criteria` |
| `TC-T2-F08-02` | **Sub-Second Dead Drop Expiry** | Seal archive key with TTL=1s | Key disappears exactly after 1 second (EXISTS == 0) | `ORIGINAL_REQUEST.md Section R4 (Scenario 4)` |
| `TC-T2-F08-03` | **Overwriting Sealed Archive Rejection** | Attempt second SET on existing archive:{id} with NX flag | Redis rejects overwrite preserving immutability of sealed output | `docs/contracts.md Section 1.3` |
| `TC-T2-F08-04` | **Empty Output Payload Sealing** | Task completes with empty result object {} | Valid SHA-256 generated for empty JSON object canonical bytes | `docs/contracts.md Section 2.C` |
| `TC-T2-F08-05` | **Gigantic Output Payload Sealing** | Seal 20MB output payload in Dead Drop | Payload stored and checksum verified without heap exhaustion | `PROJECT.md Section Interface Contracts` |

### Feature F09 Boundary & Corner: Worker Heartbeat Emitter (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `worker:{id}:heartbeat`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F09-01` | **Abrupt SIGKILL Heartbeat Expiry** | Kill worker process with SIGKILL; inspect heartbeat key over 35s | Key automatically evicted from Redis after 30 seconds TTL | `ORIGINAL_REQUEST.md Section R2` |
| `TC-T2-F09-02` | **Heartbeat Goroutine Redis Reconnection** | Disconnect Redis for 15 seconds then reconnect | Heartbeat goroutine re-establishes connection and resumes pulses | `services/worker-engine` |
| `TC-T2-F09-03` | **Special Characters in Worker ID** | Configure worker with ID worker-node_01.prod-east | Heartbeat key created and parsed correctly without regex error | `docs/contracts.md Section 1.4` |
| `TC-T2-F09-04` | **High Density Worker Fleet Heartbeats** | 50 simulated workers emit heartbeats every 10s | Redis handles 5 ops/sec without latency spikes | `PROJECT.md Section Milestones (M1)` |
| `TC-T2-F09-05` | **Clock Drift in Heartbeat Timestamp** | Worker machine clock is 5 seconds ahead of control plane clock | Wellness monitor uses Redis TTL rather than client timestamp for liveness | `ORIGINAL_REQUEST.md Section R2` |

### Feature F10 Boundary & Corner: Event Publisher (Worker Engine)
- **Target Milestone**: M1 | **Primary Interface**: `coldharbor:events Pub/Sub`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F10-01` | **Zero Subscriber Drop Immunity** | Publish 100 events when no active subscribers exist on coldharbor:events | Worker continues execution without error or memory buildup | `docs/contracts.md Section 1.5` |
| `TC-T2-F10-02` | **Event Burst Stress Handling** | Publish 1000 events in 1 second across multiple workers | Pub/Sub throughput maintained without message truncation | `ORIGINAL_REQUEST.md Section R1` |
| `TC-T2-F10-03` | **Massive Error Stack in Event Details** | Task panics with 64KB stack trace string in details | Event JSON safely serialized and published | `docs/contracts.md Section 2.B` |
| `TC-T2-F10-04` | **Pub/Sub Buffer Overflow Protection** | Subscriber client artificially throttled with slow read socket | Redis drops slow client or buffers up to client-output-buffer-limit safely | `docs/contracts.md Section 1.5` |
| `TC-T2-F10-05` | **Event Timestamp Chronological Order** | Inspect sequence of 10 rapid events from same compartment | Timestamps are strictly non-decreasing | `docs/contracts.md Section 2.B` |

### Feature F11 Boundary & Corner: Context Clearance Proxy (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `X-Context INNIE vs OUTIE`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F11-01` | **Invalid Context Header Value** | POST /api/v1/compartments with X-Context: UNKNOWN | HTTP 400 Bad Request: Invalid context clearance value | `docs/contracts.md Section 4.1` |
| `TC-T2-F11-02` | **Case Insensitivity Normalization** | POST /api/v1/compartments with X-Context: innie | Normalized to INNIE or handled cleanly according to security contract | `docs/contracts.md Section 4.1` |
| `TC-T2-F11-03` | **Header Spoofing Attack Prevention** | Inject conflicting headers X-Forwarded-Context vs X-Context | Filter strictly honors authenticated clearance contract | `ORIGINAL_REQUEST.md Section R2` |
| `TC-T2-F11-04` | **Cross-Context Data Leak in Audit Query** | GET /api/v1/audits with X-Context: OUTIE | Response strictly excludes records belonging to INNIE context | `docs/contracts.md Section 4.1` |
| `TC-T2-F11-05` | **Empty Header Value Rejection** | POST /api/v1/compartments with X-Context: empty string | HTTP 400 Bad Request rejecting empty clearance | `docs/contracts.md Section 4.1` |

### Feature F12 Boundary & Corner: Compartment REST API (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `/api/v1/compartments/**`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F12-01` | **Missing Mandatory Field in Submission** | POST /api/v1/compartments with payload missing taskType | HTTP 400 Bad Request with validation error message | `PROJECT.md Section Interface Contracts (5)` |
| `TC-T2-F12-02` | **Empty HTTP Request Body** | POST /api/v1/compartments with empty body | HTTP 400 Bad Request with body required message | `PROJECT.md Section Interface Contracts (5)` |
| `TC-T2-F12-03` | **Oversized Request Body Payload** | POST /api/v1/compartments with 50MB payload | HTTP 413 Payload Too Large rejecting excessive size | `PROJECT.md Section Interface Contracts (5)` |
| `TC-T2-F12-04` | **SQL Injection in Compartment ID URL** | GET /api/v1/compartments/malformed-id-with-sql-quotes | HTTP 400 or 404 handled cleanly with parameterized query safety | `docker/init-db.sql` |
| `TC-T2-F12-05` | **Premature Dead Drop Query** | GET /api/v1/compartments/{id}/deaddrop while still in RUNNING | HTTP 404 Not Found indicating output not yet archived | `docs/contracts.md Section 1.3` |

### Feature F13 Boundary & Corner: Redis Stream Producer (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `XADD coldharbor:jobs`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F13-01` | **Redis Connection Failure on Dispatch** | Stop Redis and attempt POST /api/v1/compartments | Returns HTTP 503 Service Unavailable cleanly without crashing control plane | `PROJECT.md Section Milestones (M2)` |
| `TC-T2-F13-02` | **Stream Capping MAXLEN Enforcement** | Configure stream producer with MAXLEN ~ 10000 | XADD includes MAXLEN parameter ensuring bounded Redis memory | `docs/contracts.md Section 1.1` |
| `TC-T2-F13-03` | **Concurrent High Rate Job Dispatch** | Send 100 concurrent POST requests from 10 client threads | All 100 jobs successfully published with unique monotonic IDs | `ORIGINAL_REQUEST.md Section R2` |
| `TC-T2-F13-04` | **Non-ASCII Unicode in Task Payload** | Submit task containing multi-byte Japanese and emoji characters | Stream message safely encodes and decodes UTF-8 without corruption | `docs/contracts.md Section 2.A` |
| `TC-T2-F13-05` | **Payload Size Boundary Validation** | Submit task payload at exact boundary (1MB) | Producer accepts payload within limits and rejects payloads exceeding threshold | `PROJECT.md Section Interface Contracts` |

### Feature F14 Boundary & Corner: PostgreSQL Audit Service (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `audit_records table`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F14-01` | **Database Constraint Context Check** | Attempt INSERT into audit_records with context=INVALID | PostgreSQL check constraint audit_records_context_check rejects insert with SQL state 23514 | `docker/init-db.sql` |
| `TC-T2-F14-02` | **Database Constraint Final State Check** | Attempt INSERT into audit_records with final_state=RUNNING | PostgreSQL check constraint audit_records_final_state_check rejects insert with SQL state 23514 | `docker/init-db.sql` |
| `TC-T2-F14-03` | **Timestamp Range Boundary Queries** | Query GET /api/v1/audits with from and to set to exact created_at timestamp | Returns exact matching record without off-by-one errors | `PROJECT.md Section Interface Contracts (5)` |
| `TC-T2-F14-04` | **High Offset Pagination Performance** | Query GET /api/v1/audits with offset=1000000 | Returns empty array HTTP 200 OK within 50ms using indexed scan | `docker/init-db.sql` |
| `TC-T2-F14-05` | **SQL Injection in Filter Params** | Query GET /api/v1/audits?ownerId=1;DROP TABLE audit_records;-- | Parameterized query safely treats input as literal string; database unaffected | `docker/init-db.sql` |

### Feature F15 Boundary & Corner: WebSocket Event Gateway (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `/ws/events endpoint`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F15-01` | **Context-Aware WebSocket Event Filtering** | OUTIE client connects to /ws/events; worker emits INNIE event | OUTIE client socket does not receive INNIE event frame | `ORIGINAL_REQUEST.md Section R2, docs/contracts.md Section 4.1` |
| `TC-T2-F15-02` | **Slow WebSocket Consumer Handling** | Client stops reading WebSocket frames; producer emits 1000 events | Gateway buffers up to limit then disconnects slow client without crashing server | `PROJECT.md Section Milestones (M2)` |
| `TC-T2-F15-03` | **Abrupt Client Connection Drop** | Client closes TCP connection without WS close frame | Gateway cleans up WebSocket session and unregisters subscriber cleanly | `services/control-plane` |
| `TC-T2-F15-04` | **WebSocket Heartbeat Ping-Pong** | Observe WebSocket connection over 2 minutes of inactivity | Ping/Pong control frames exchanged to keep TCP connection alive | `services/control-plane` |
| `TC-T2-F15-05` | **Unauthorized WebSocket Handshake** | Connect to /ws/events without required authentication token or clearance | Handshake rejected with HTTP 401 Unauthorized or 403 Forbidden | `docs/contracts.md Section 4.1` |

### Feature F16 Boundary & Corner: Worker Wellness Monitor (Control Plane)
- **Target Milestone**: M2 | **Primary Interface**: `worker:*:heartbeat scanner`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F16-01` | **High Worker Density Scanning** | Simulate 100 worker heartbeat keys in Redis | Wellness scanner iterates through all keys via SCAN without blocking Redis | `PROJECT.md Section Milestones (M2)` |
| `TC-T2-F16-02` | **Worker Heartbeat Flapping Detection** | Worker pulses heartbeats intermittently with 28s gaps | Wellness monitor maintains stable status without flip-flopping alert state | `ORIGINAL_REQUEST.md Section R2` |
| `TC-T2-F16-03` | **Redis SCAN Cursor Pagination** | Set SCAN match pattern across multi-page key results | Scanner handles non-zero cursors until cursor returns 0 without missing keys | `services/control-plane` |
| `TC-T2-F16-04` | **Clock Skew Tolerance** | Worker system clock differs from control plane by 10s | Wellness monitor evaluates Redis TTL rather than payload timestamp for liveness | `ORIGINAL_REQUEST.md Section R2` |
| `TC-T2-F16-05` | **Corrupt Heartbeat JSON Recovery** | Inject invalid non-JSON string into worker:rogue:heartbeat | Scanner logs warning, flags worker suspicious, and continues scanning without crash | `services/control-plane` |

### Feature F17 Boundary & Corner: Worker Telemetry UI (Dashboard)
- **Target Milestone**: M3 | **Primary Interface**: `WorkerTelemetry component`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F17-01` | **Large Fleet Virtual Scrolling** | API returns 200 active workers | WorkerTelemetry renders virtualized list maintaining 60fps scrolling | `ORIGINAL_REQUEST.md Section R3` |
| `TC-T2-F17-02` | **Rapid Status Toggle Rendering** | Worker toggles between IDLE and BUSY 5 times per second | UI animates state transition smoothly without DOM thrashing | `web/src/components/WorkerTelemetry.tsx` |
| `TC-T2-F17-03` | **Network Disconnect Retry State** | REST API calls fail with network error | WorkerTelemetry displays Retry connection banner with auto-retry countdown | `web/src/components/WorkerTelemetry.tsx` |
| `TC-T2-F17-04` | **String Overflow Ellipsis** | Worker configured with 64-character UUID and long compartment name | UI truncates cleanly with ellipsis and tooltip on hover | `web/src/components/WorkerTelemetry.tsx` |
| `TC-T2-F17-05` | **Stale Telemetry Indicator** | No updates received from server for 30s | Component displays Stale data warning badge | `ORIGINAL_REQUEST.md Section R3` |

### Feature F18 Boundary & Corner: Compartment Lifecycle Tracker (Dashboard)
- **Target Milestone**: M3 | **Primary Interface**: `LifecycleTracker component`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F18-01` | **Direct Jump to FAILED Display** | Compartment fails immediately during validation (CREATED -> FAILED) | Lifecycle tracker renders red failure path bypassing intermediate nodes | `ORIGINAL_REQUEST.md Section R3` |
| `TC-T2-F18-02` | **Sub-Millisecond Checkpoint Replay** | Event history contains 3 checkpoints occurring within 10ms | Tracker animates progress smoothly to 100% without getting stuck | `web/src/components/LifecycleTracker.tsx` |
| `TC-T2-F18-03` | **Fast Compartment Switching** | User clicks between 5 different compartments in quick succession | Tracker cancels pending state updates and renders active compartment correctly | `web/src/components/LifecycleTracker.tsx` |
| `TC-T2-F18-04` | **Non-Existent Compartment Query** | Pass non-existent compartment ID to LifecycleTracker | Displays Compartment not found notice without crashing UI tree | `web/src/components/LifecycleTracker.tsx` |
| `TC-T2-F18-05` | **Small Screen Responsive Layout** | Render tracker in 375px mobile viewport | FSM nodes wrap or scale responsively without horizontal clipping | `ORIGINAL_REQUEST.md Section R3` |

### Feature F19 Boundary & Corner: Live WebSocket Event Feed (Dashboard)
- **Target Milestone**: M3 | **Primary Interface**: `EventFeed component`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F19-01` | **High Rate Event Throttling** | Server sends 200 events/sec over WebSocket | EventFeed batches UI updates with requestAnimationFrame preventing UI lockup | `web/src/components/EventFeed.tsx` |
| `TC-T2-F19-02` | **Event Feed Buffer Memory Cap** | Stream 10,000 events into feed over 10 minutes | Buffer maintains maximum 500 entries, evicting oldest to prevent memory growth | `ORIGINAL_REQUEST.md Section R3` |
| `TC-T2-F19-03` | **Reconnection State Banner** | Drop WebSocket connection and reconnect | Banner transitions Connected -> Reconnecting... -> Connected cleanly | `ORIGINAL_REQUEST.md Section Acceptance Criteria` |
| `TC-T2-F19-04` | **Event Search Filtering Stress** | Filter feed with regex pattern across 500 items | Search filters results in real-time with debounce (<50ms) | `web/src/components/EventFeed.tsx` |
| `TC-T2-F19-05` | **Corrupt Event Frame Handling** | Inject non-JSON text frame into WebSocket stream | EventFeed catches JSON parse error, logs warning, and keeps feed running | `web/src/components/EventFeed.tsx` |

### Feature F20 Boundary & Corner: Dead Drop Inspector UI (Dashboard)
- **Target Milestone**: M3 | **Primary Interface**: `DeadDropInspector component`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F20-01` | **Expired Dead Drop Inspection** | Query Dead Drop after TTL has elapsed (404 response) | Inspector displays Dead Drop expired and purged with link to audit log | `ORIGINAL_REQUEST.md Section R3` |
| `TC-T2-F20-02` | **Checksum Tampering Detection** | Simulate modified Dead Drop where SHA-256 does not match output | Inspector displays red Checksum Mismatch Alert badge | `ORIGINAL_REQUEST.md Section Acceptance Criteria` |
| `TC-T2-F20-03` | **Deep Nested JSON Viewer** | Load Dead Drop with 10 levels of nested JSON objects | Collapsible tree viewer renders structure with expand/collapse controls | `web/src/components/DeadDropInspector.tsx` |
| `TC-T2-F20-04` | **Binary Base64 Payload Display** | Load Dead Drop containing base64 cipher stream | Viewer provides Base64 decode toggle and hex preview | `PROJECT.md Section Interface Contracts` |
| `TC-T2-F20-05` | **HTML Injection Protection** | Dead Drop payload contains <script>alert(1)</script> | Text rendered using safe textContent / React escaping without execution | `ORIGINAL_REQUEST.md Section R3` |

### Feature F21 Boundary & Corner: Historical Audit Browser (Dashboard)
- **Target Milestone**: M3 | **Primary Interface**: `AuditBrowser component`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F21-01` | **Zero Search Results Handling** | Search audit logs for non-existent compartment ID | Table displays No historical audit records match criteria gracefully | `ORIGINAL_REQUEST.md Section R3` |
| `TC-T2-F21-02` | **Partial Compartment ID Matching** | Search audit logs with partial UUID prefix cpt_a1b2 | Returns all matching records with highlighted matching text | `ORIGINAL_REQUEST.md Section R3` |
| `TC-T2-F21-03` | **Multi-Column Audit Sorting** | Sort table by Duration ascending then CompletedAt descending | Table updates ordering correctly according to selected sort criteria | `web/src/components/AuditBrowser.tsx` |
| `TC-T2-F21-04` | **Large Dataset CSV Export** | Click Export CSV with 1000 audit records loaded | Generates valid downloadable CSV file matching table columns | `web/src/components/AuditBrowser.tsx` |
| `TC-T2-F21-05` | **Complex JSONB Metadata Modal** | Audit record contains extensive worker checkpoint telemetry in metadata | Modal renders formatted JSON with search and copy functionality | `ORIGINAL_REQUEST.md Section R3` |

### Feature F22 Boundary & Corner: Local Infra Orchestration (Infrastructure)
- **Target Milestone**: M4 | **Primary Interface**: `docker-compose.yml Redis & PG`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F22-01` | **Container Restart Persistence** | Restart coldharbor-redis container with docker-compose restart redis | AOF file retains existing stream messages and keys upon reboot | `docker/docker-compose.yml` |
| `TC-T2-F22-02` | **Clean Down and Volume Reset** | Execute docker-compose down -v | Volumes coldharbor_redis_data and coldharbor_pg_data removed cleanly | `docker/docker-compose.yml` |
| `TC-T2-F22-03` | **Port Conflict Error Handling** | Simulate port 6379 already bound by host process | docker-compose logs clear port binding error without unhandled kernel panic | `docker/docker-compose.yml` |
| `TC-T2-F22-04` | **Database Configuration Integrity** | Verify POSTGRES_DB, USER, PASSWORD match control plane application.yml | Control plane successfully connects to PostgreSQL without auth failure | `docker/docker-compose.yml` |
| `TC-T2-F22-05` | **Container Memory Limit Ceiling** | Apply container resource constraints under high task load | Containers operate stably without OOM killer termination | `PROJECT.md Section Milestones (M4)` |

### Feature F23 Boundary & Corner: E2E Verification Suite (Quality Assurance)
- **Target Milestone**: M4 | **Primary Interface**: `scripts/verify_e2e.ps1`

| Test ID | Test Name | Boundary / Stress Input | Expected Error Handling & Invariant | Authoritative Source |
|---|---|---|---|---|
| `TC-T2-F23-01` | **Custom Endpoint Override** | Execute scripts/verify_e2e.ps1 -TargetHost staging.internal -TargetPort 8443 | Script directs REST and protocol requests to specified custom host and port | `ORIGINAL_REQUEST.md Section R4` |
| `TC-T2-F23-02` | **Mock Mode Standalone Execution** | Execute scripts/verify_e2e.ps1 -MockMode | Harness validates protocol schemas and state contracts in isolated offline mode | `PROJECT.md Section Milestones (M4)` |
| `TC-T2-F23-03` | **Diagnostic Failure Output** | Simulate assertion failure on Scenario 5 (scratchpad leak) | Harness outputs exact failed key, expected EXISTS=0, actual EXISTS=1 | `ORIGINAL_REQUEST.md Section R4` |
| `TC-T2-F23-04` | **Strict Non-Zero Exit Code** | Run verify_e2e.ps1 against failing mock environment | Process returns exit code 1; verifiable by  in CI | `ORIGINAL_REQUEST.md Section Acceptance Criteria` |
| `TC-T2-F23-05` | **Summary Table Output Formatting** | Inspect final stdout of verify_e2e.ps1 | Prints formatted console table with columns: Scenario, Status, ElapsedTime, Details | `ORIGINAL_REQUEST.md Section R4` |

---

## 6. Tier 3: Pairwise Combination Suite (15 Test Cases)

Tier 3 validates orthogonal cross-subsystem interactions, ensuring that contracts between independently developed services interface seamlessly.

| Test ID | Pairwise Combination Name | Subsystems Interfacing | Test Scenario Description | Expected Observable Outcome |
|---|---|---|---|---|
| `TC-T3-01` | **REST Dispatch to Worker Consumer** | Stream Producer (F13) + Consumer Group (F01) | Post job via REST API; verify Go worker consumer group consumes message from coldharbor:jobs | Message transferred from control plane to worker stream PEL without data corruption |
| `TC-T3-02` | **Consumer Failures to DLQ Routing** | Consumer Group (F01) + Dead-Letter Queue (F02) | Worker fails task 3 times; verify transition to DLQ and XACK on primary stream | Primary stream PEL count decrements by 1; message appears in coldharbor:jobs:dlq |
| `TC-T3-03` | **State Transitions with Scratchpad Commits** | Finite State Machine (F03) + Scratchpad Memory (F04) | FSM advances through CHECKPOINT states; verify intermediate progress commits to Redis Hash | Each FSM transition updates current_step and progress_pct in compartment:{id}:mem |
| `TC-T3-04` | **Checkpoint Storage and PEL Resumption** | Multi-Step Checkpointing (F06) + Crash Recovery (F07) | Simulate worker crash at 50% milestone; successor claims job via XAUTOCLAIM | Successor recovers scratchpad state at step 2 and resumes at step 3 without repeating steps 1-2 |
| `TC-T3-05` | **Recovered Job Completion and Purge** | Crash Recovery (F07) + Atomic Purge (F05) | Successor worker executes recovered job to completion | Scratchpad hash deleted via atomic DEL (EXISTS==0) before acknowledging message with XACK |
| `TC-T3-06` | **Completed Output Sealing with SHA-256** | Finite State Machine (F03) + Dead Drop Archive (F08) | Task enters COMPLETED state; worker seals archive:{id} with canonical SHA-256 checksum | archive:{id} contains immutable output and valid SHA-256 seal with configured TTL |
| `TC-T3-07` | **Dead Drop Sealing to PostgreSQL Audit** | Dead Drop Archive (F08) + PostgreSQL Audit (F14) | Task completes and seals Dead Drop; control plane records audit record in PostgreSQL | checksum and duration_ms in audit_records table match Dead Drop archive exactly |
| `TC-T3-08` | **Worker Heartbeat Pulse to Wellness Monitor** | Worker Heartbeat (F09) + Wellness Monitor (F16) | Worker emits pulse every 10s; control plane wellness monitor queries worker:*:heartbeat | Control plane worker registry reflects worker status ACTIVE and tracks liveness |
| `TC-T3-09` | **Wellness Monitor Dead Worker to Stream Recovery** | Wellness Monitor (F16) + Crash Recovery (F07) | Worker process killed; heartbeat expires (>30s); wellness monitor flags worker DEAD | Pending stream messages owned by dead worker reclaimed by standby worker via XCLAIM |
| `TC-T3-10` | **Worker Pub/Sub to WebSocket Gateway** | Event Publisher (F10) + WebSocket Gateway (F15) | Worker publishes state transitions to coldharbor:events; gateway relays to WS subscribers | Connected WebSocket clients receive real-time JSON frames within 50ms |
| `TC-T3-11` | **WebSocket Gateway to Dashboard Event Feed** | WebSocket Gateway (F15) + Event Feed UI (F19) | Stream transition events through WebSocket into React EventFeed component | EventFeed renders new event cards dynamically in reverse-chronological order |
| `TC-T3-12` | **Context Clearance to Dead Drop Retrieval** | Context Proxy (F11) + Compartment REST API (F12) | OUTIE client attempts to fetch Dead Drop of an INNIE compartment | REST API returns HTTP 403 Forbidden preventing cross-context information leakage |
| `TC-T3-13` | **Context Clearance to PostgreSQL Audit Filtering** | Context Proxy (F11) + PostgreSQL Audit (F14) | OUTIE client queries GET /api/v1/audits | Returned records filtered strictly to context OUTIE, omitting all INNIE records |
| `TC-T3-14` | **Dead Drop Inspector UI to REST Endpoint** | Dead Drop Inspector (F20) + Compartment REST API (F12) | User queries compartment in DeadDropInspector UI component | UI renders formatted payload, verified SHA-256 badge, and active TTL countdown |
| `TC-T3-15` | **Docker Compose Infrastructure to E2E Harness** | Local Infrastructure (F22) + E2E Verification Suite (F23) | Execute verify_e2e.ps1 against live docker-compose services (Redis 7.2 + PostgreSQL 16) | All 6 mandatory integration scenarios pass cleanly with exit code 0 |

---

## 7. Tier 4: Real-World Application Scenarios (10 Test Cases)

Tier 4 exercises full-system integration flows, including the **6 Mandatory Integration Scenarios** defined in `ORIGINAL_REQUEST.md Â§R4`.

### TC-T4-01: R4 Scenario 1: Job Submission via REST API
- **Authoritative Source**: `ORIGINAL_REQUEST.md Section R4.1`
- **Execution Preconditions & Input**:
  Submit POST /api/v1/compartments with context INNIE, taskType DATA_REDUCTION, and payload
- **Expected Observable Output & Verification Criteria**:
  - 1. API validates schema and context clearance
2. Compartment record created with status QUEUED
3. Structured JobMessage published to coldharbor:jobs via XADD
4. API returns HTTP 201 Created with compartmentId

### TC-T4-02: R4 Scenario 2: Worker Claim, Scratchpad & Checkpoints
- **Authoritative Source**: `ORIGINAL_REQUEST.md Section R4.2`
- **Execution Preconditions & Input**:
  Worker consumes job from coldharbor:jobs via consumer group worker-group
- **Expected Observable Output & Verification Criteria**:
  - 1. Worker transitions state from QUEUED to RUNNING
2. Initializes ephemeral scratchpad hash compartment:{id}:mem
3. Executes step 1 (25%), writes intermediate state to scratchpad, emits event
4. Executes step 2 (50%), updates scratchpad, emits event
5. Executes step 3 (75%), updates scratchpad, emits event

### TC-T4-03: R4 Scenario 3: Worker Crash Simulation & Recovery
- **Authoritative Source**: `ORIGINAL_REQUEST.md Section R4.3`
- **Execution Preconditions & Input**:
  Simulate abrupt worker termination (SIGKILL) immediately after committing checkpoint at step 2 (50%)
- **Expected Observable Output & Verification Criteria**:
  - 1. Worker process terminates leaving message unacknowledged in PEL
2. Standby worker detects idle message and claims via XAUTOCLAIM
3. Standby reads compartment:{id}:mem, discovers current_step=2 (50% complete)
4. Resumes calculation directly at step 3, skipping steps 1 and 2
5. Completes remaining calculation successfully

### TC-T4-04: R4 Scenario 4: Dead Drop Sealing with SHA-256 & TTL
- **Authoritative Source**: `ORIGINAL_REQUEST.md Section R4.4`
- **Execution Preconditions & Input**:
  Task execution completes; worker seals result into Dead Drop archive
- **Expected Observable Output & Verification Criteria**:
  - 1. Worker computes canonical SHA-256 hexadecimal hash over task output
2. Stores sealed JSON into Redis key archive:{compartmentId} with TTL 3600s
3. Retrieves key to verify SHA-256 matches exact output fingerprint
4. Verifies TTL countdown is active in Redis

### TC-T4-05: R4 Scenario 5: Atomic Scratchpad Deletion (Zero Leak)
- **Authoritative Source**: `ORIGINAL_REQUEST.md Section R4.5`
- **Execution Preconditions & Input**:
  Worker enters PURGED state after sealing Dead Drop
- **Expected Observable Output & Verification Criteria**:
  - 1. Worker issues atomic DEL compartment:{compartmentId}:mem
2. Query EXISTS compartment:{compartmentId}:mem returns exactly 0
3. Keyspace scan KEYS compartment:*:mem confirms zero residual keys
4. Worker issues XACK to acknowledge stream message

### TC-T4-06: R4 Scenario 6: Durable PostgreSQL Audit Persistence
- **Authoritative Source**: `ORIGINAL_REQUEST.md Section R4.6`
- **Execution Preconditions & Input**:
  Compartment execution reaches terminal PURGED state
- **Expected Observable Output & Verification Criteria**:
  - 1. Control Plane inserts immutable record into audit_records
2. Record contains UUID, compartment_id, owner_id, context, final_state=PURGED
3. duration_ms accurately recorded (completed_at - created_at)
4. checksum matches exact SHA-256 fingerprint in Dead Drop
5. Query SELECT * FROM audit_records WHERE compartment_id=:id confirms persistence

### TC-T4-07: Concurrent Multi-Worker Load Partitioning
- **Authoritative Source**: `PROJECT.md Section Architecture`
- **Execution Preconditions & Input**:
  Dispatch 10 concurrent compartments across 3 active Go workers
- **Expected Observable Output & Verification Criteria**:
  - 1. Jobs distributed across workers via worker-group
2. All 10 compartments execute with isolated scratchpads
3. All 10 Dead Drops sealed with unique SHA-256 checksums
4. All 10 scratchpads purged with zero residual memory

### TC-T4-08: Poison Pill Isolation & DLQ Routing Under Load
- **Authoritative Source**: `ORIGINAL_REQUEST.md Section R1`
- **Execution Preconditions & Input**:
  Submit invalid corrupted payload concurrently with 5 valid jobs
- **Expected Observable Output & Verification Criteria**:
  - 1. Valid jobs execute smoothly to PURGED state
2. Corrupt job fails, retries up to maxRetries (3), and transfers to coldharbor:jobs:dlq
3. Primary stream message XACKed; scratchpad purged; failure recorded in PostgreSQL

### TC-T4-09: Full Lifecycle Telemetry & UI Synchronization
- **Authoritative Source**: `ORIGINAL_REQUEST.md Section R3`
- **Execution Preconditions & Input**:
  Execute complete compartment lifecycle with live WebSocket client attached
- **Expected Observable Output & Verification Criteria**:
  - 1. Client receives real-time events for all state transitions
2. Lifecycle tracker highlights each state in sequence
3. Telemetry reflects worker status BUSY then IDLE
4. Dead Drop inspector retrieves verified output upon completion

### TC-T4-10: Zero-Trust Security Boundary Enforcement
- **Authoritative Source**: `docs/contracts.md Section 4.1`
- **Execution Preconditions & Input**:
  Submit INNIE compartment and OUTIE compartment; execute cross-context queries
- **Expected Observable Output & Verification Criteria**:
  - 1. INNIE client successfully accesses INNIE compartment and audit log
2. OUTIE client successfully accesses OUTIE compartment
3. OUTIE client blocked (HTTP 403) from accessing INNIE compartment or audit log
4. SYSTEM/ADMIN clearance allowed cross-boundary access

---

## 8. Verification Harness Architecture & Execution Runner

### 8.1 Automated Test Runner (`scripts/verify_e2e.ps1`)
The primary verification harness is implemented in PowerShell (`scripts/verify_e2e.ps1`). It provides automated execution of all 6 mandatory integration scenarios plus Tier 1-4 contract checks.

#### Runner Capabilities:
1. **Live Environment Mode**: Automatically detects running Docker containers or local service instances on ports 8080 (Control Plane), 6379 (Redis), and 5432 (PostgreSQL).
2. **Standalone Mock Verification Mode (`-MockMode`)**: Validates protocol contracts, Redis keyspace invariants, SHA-256 cryptographic sealing, and JSON schemas offline using pure PowerShell & Go/Node standard tooling.
3. **Configurable Endpoints**: Accepts `-TargetHost`, `-TargetPort`, `-RedisPort`, and `-PostgresPort` for staging or remote verification.
4. **Comprehensive Exit Codes**: Returns exit code 0 on 100% scenario pass; returns exit code 1 on any assertion failure for strict CI integration.

#### Command Execution Examples:
```powershell
# Execute standard E2E verification against local environment
powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1

# Execute standalone contract validation mode (offline / CI)
powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1 -MockMode

# Execute targeted verification against custom staging endpoints
powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1 -TargetHost staging.internal -TargetPort 8080
```

### 8.2 Scenario-to-Verification Mapping
| Mandatory Scenario | Verification Method in `verify_e2e.ps1` | Invariant Proven |
|---|---|---|
| **Scenario 1**: Job Submission via REST | HTTP POST to `/api/v1/compartments` + Stream inspect | HTTP 201 Created, compartment created with status `QUEUED`, stream entry added |
| **Scenario 2**: Worker Claim & Checkpoints | Monitor `compartment:{id}:mem` and `coldharbor:events` | Worker transitions to `RUNNING`, scratchpad initialized, 3 checkpoints emitted (25%, 50%, 75%) |
| **Scenario 3**: Worker Crash & Recovery | Simulate worker kill at step 2, standby claims PEL | Successor resumes from step 3 without repeating step 1 or 2 |
| **Scenario 4**: Dead Drop Sealing & TTL | Query Redis `archive:{id}` | JSON payload sealed with exact SHA-256 checksum and positive TTL |
| **Scenario 5**: Atomic Purge (Zero Leak) | Query Redis `EXISTS compartment:{id}:mem` | Returns `0` (non-existent). Zero residual keys in `KEYS compartment:*:mem` |
| **Scenario 6**: Durable PostgreSQL Audit | Query PostgreSQL `audit_records` table | Row persisted with matching UUID, status `PURGED`, duration, and SHA-256 checksum |


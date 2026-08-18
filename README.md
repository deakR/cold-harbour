# ColdHarbor

> **A Distributed, Compartmentalized Task Execution Platform**  
> *Memory is temporary. Results are permanent. Context must never leak.*

---

## Overview

**ColdHarbor** is a distributed execution engine built on the principles of execution isolation, ephemeral memory, fault-tolerant checkpointing, and immutable outcomes. 

Every task runs in an isolated **Compartment** with strict security boundaries (`INNIE` / `OUTIE`). During execution, intermediate calculations are written to temporary scratchpad memory. Upon task completion or failure, all runtime scratchpad memory is permanently destroyed—leaving only a cryptographically signed output in a self-destructing Dead Drop archive and a durable audit record.

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

- **Spring Boot 3 (Java 21)**: Control plane, JWT authentication, context isolation proxy, compartment lifecycle manager, REST endpoints, and WebSocket event gateway.
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
Intermediate state is held in Redis Hashes (`compartment:{id}:mem`). When a job terminates (success or failure), the scratchpad is destroyed via an atomic `DEL` command. Zero memory footprint remains.

### 3. Checkpointing & Crash Resilience
Workers record periodic progress checkpoints (e.g., 25%, 50%, 75%). If a worker crashes mid-task, another worker claims the unacknowledged message from the Redis Stream consumer group and resumes execution from the latest checkpoint.

### 4. Dead Drop Archive
Completed task outputs are sealed with a cryptographic **SHA-256 checksum** and stored with an expiring **TTL** in Redis (`archive:{id}`).

### 5. Dual-Tier Persistence
- **Hot / Ephemeral (Redis)**: Milliseconds to hours lifespan for active queues, scratchpads, and dead drops.
- **Cold / Permanent (PostgreSQL)**: Durable `audit_records` table storing completion metadata, timestamps, and integrity checksums.

---

## Project Structure

```text
coldharbour/
├── docker/
│   ├── docker-compose.yml     # Redis 7 + PostgreSQL 16 container definitions
│   └── init-db.sql            # Audit table DDL and indices
├── docs/
│   └── contracts.md           # Redis schemas, payloads, and state transitions
├── services/
│   ├── control-plane/         # Spring Boot 3 Control Plane (Java 21)
│   └── worker-engine/         # Go Worker Engine (Go 1.25+)
├── web/                       # React Dashboard (Vite + TypeScript)
└── README.md
```

---

## Quickstart (Infrastructure)

### Prerequisites
- [Docker & Docker Compose](https://www.docker.com/)
- [Go 1.22+](https://go.dev/)
- [Java 21+ & Maven/Gradle](https://adoptium.net/)
- [Node.js 20+](https://nodejs.org/)

### 1. Start Infrastructure Services
```bash
docker compose -f docker/docker-compose.yml up -d
```

### 2. Verify Services
- **Redis**: Port `6379`
- **PostgreSQL**: Port `5432` (Database: `coldharbor`, User: `coldharbor`)

---

## Documentation
- [System Contracts & Protocol Specification](docs/contracts.md)

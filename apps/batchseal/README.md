# BatchSeal — Verifiable Batch Computation for Desktop

BatchSeal is a Wails (Go + React) desktop app that turns ColdHarbor into a
daily-use tool: paste numbers, seal the batch, keep the receipt.

Each batch runs in an isolated ColdHarbor compartment with checkpointing. The
finished result is sealed with SHA-256, verified locally inside the app, and
stored as an offline receipt copy. The durable audit trail stays queryable
through the control plane.

## Run

Prerequisites: Go 1.25+, Node 22+, and a live ColdHarbor stack
(`make stack` from the repository root, or control plane at
`http://localhost:8080` with Redis and a Go worker).

```bash
cd apps/batchseal
wails dev      # hot-reload desktop shell
wails build    # produces build/bin/batchseal.exe
```

Backend-only verification (standard library, no GUI needed):

```bash
cd apps/batchseal
go test ./backend/
```

## Use

1. **New batch**: pick `DATA_REDUCTION` (totals), `CIPHER_STREAM` (digest),
   or `ARCHIVE_SEAL` (sealed count). Paste values comma/space/line separated.
   The crash-probe toggle dispatches with `simulateCrashAtStep: 2` to watch
   PEL checkpoint recovery live.
2. **Live**: batches poll compartment state every 2s. On terminal state the
   app fetches the Dead Drop, recomputes SHA-256 over the canonical output,
   and saves a receipt copy to the OS config dir (`batchseal/receipts/`).
3. **Receipts**: local sealed copies plus the server-side durable audit.
4. **Fleet**: worker liveness and dead-letter depth. `Redrive 10` requeues
   dead letters for another attempt via `POST /api/v1/workers/dlq/redrive`.
5. **Settings**: base URL, API key (`X-API-Key`), clearance, owner ID.
   Persisted to `config.json` (mode 0600) in the app config dir.

## Design notes

- No new Go dependencies: REST over `net/http` with polling instead of a
  websocket client library, seal checks with `crypto/sha256`, storage with
  `os`/`encoding/json`. The only added module is Wails itself.
- Seal verification mirrors the worker's `ComputeChecksum`: `encoding/json`
  sorts map keys, so marshaling the decoded output reproduces the sealed
  bytes exactly. A mismatch means tampering or a canonicalization break,
  never silent acceptance.
- Receipts are immutable local evidence: if a Dead Drop TTL expires, the
  local copy and the PostgreSQL audit row remain.

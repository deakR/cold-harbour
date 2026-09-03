# ColdHarbor Threat Model

## Trust boundaries

1. Client to control plane. Untrusted network. Authentication is by `X-API-Key`.
   The self-asserted `X-Context-Clearance` header is accepted only when no API
   keys are configured or `coldharbor.security.allow-unsafe-header=true`
   (dev/test default). Production sets keys and disables the unsafe header.
2. Control plane to Redis. Trusted VPC. Redis holds transient queues,
   scratchpads, heartbeats, and TTL Dead Drops. Compromise exposes in-flight
   payloads but not durable history.
3. Control plane to PostgreSQL. Trusted VPC. `audit_records` is the durable
   record, including full output in `metadata.output` and the SHA-256 checksum.
4. Go workers to Redis. Trusted VPC. Workers hold no credentials and keep no
   local state. Scratchpads are namespaced per compartment and purged on
   terminal state.

## Abuse cases and mitigations

| Abuse | Mitigation |
|---|---|
| Clearance spoofing via forged `X-Context-Clearance` | Valid `X-API-Key` maps to server-side clearance and takes precedence. Unknown keys return 401. With keys configured and `allow-unsafe-header=false`, header-only access returns 401. Constant-time key comparison. |
| Cross-context read (`OUTIE` reading `INNIE` compartment, dead drop, or audit) | `ContextClearance.canAccess` enforced in `CompartmentService` and `AuditService` on every read path. Violation returns 403. |
| Replay of a captured request | Trace IDs (`X-Trace-Id` generated per request when absent) propagate through logs and responses. Replay still requires a valid API key. Rotate keys by updating `coldharbor.security.api-keys`. |
| Key compromise | Keys map to least-privilege clearance. Revoke by removing the entry and restarting or refreshing config. Audit records retain which context performed each action. |
| Dead Drop expiry destroying evidence | `GET /api/v1/compartments/{id}/deaddrop` falls back to the PostgreSQL audit record after Redis TTL expiry, with `remainingTtlSeconds: 0`. Checksum comparison still applies. |
| Worker impersonation via forged heartbeat | Heartbeats are liveness hints only. Job ownership derives from Redis Stream PEL semantics (`XAUTOCLAIM` with idle timeout), not heartbeat content. |
| Scratchpad leak across compartments | Keys are namespaced `compartment:{id}:mem`. Purge is `DEL` on terminal state in success and failure branches, verified with `EXISTS == 0` before `XACK`. |

## Residual risks

- No request rate limiting. Add a gateway or Spring throttling before internet exposure.
- Redis traffic is unencrypted by default. Enable TLS in managed deployments.
- API keys are static. Rotate on a schedule. Prefer a secret manager over config files.

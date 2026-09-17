# Security policy

## Supported versions

ColdHarbor is currently a development and reference implementation. Security
fixes are applied to the latest commit on `main`; no released version is
certified for public, multi-tenant production use.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub's
**Security → Report a vulnerability** workflow for this repository so the
report and any proof of concept remain private.

Include:

- the affected endpoint, component, and commit;
- reproduction steps and required configuration;
- the expected and observed authorization boundary;
- impact, including whether data from another context is exposed.

Do not include real API keys, credentials, customer data, or destructive
payloads.

## Deployment baseline

Before deploying outside a local development environment:

- disable `COLDHARBOR_ALLOW_UNSAFE_HEADER`;
- configure server-side API keys and restrict WebSocket origins;
- keep Redis and PostgreSQL on private networks with authentication and TLS;
- replace Compose development credentials with managed secrets;
- restrict Actuator, metrics, Swagger, and DLQ operator endpoints;
- run the live verification suite against the deployed configuration.

See `docs/threat-model.md` for trust boundaries and residual risks.

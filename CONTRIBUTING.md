# Contributing to ColdHarbor

## Development setup

See the [README](README.md#quickstart) for prerequisites and local services.
Install Maven on `PATH`; the included `mvnw` launcher delegates to it.
Use development credentials only on a trusted local machine.

## Checks before a pull request

Run these commands from their respective directories:

| Directory | Commands |
| --- | --- |
| `services/worker-engine` | `go vet ./...` and `go test ./...` |
| `services/control-plane` | `mvn -B verify` |
| `web` | `npm ci`, `npm test`, and `npm run build` |

From the repository root, run the offline contract simulation:

```powershell
./scripts/verify_e2e.ps1 -MockMode
```

For integration checks, start the complete stack following the README, then run:

```powershell
./scripts/verify_e2e.ps1 -PostgresPort 5433
```

The harness can fall back to simulation when infrastructure is unavailable.
Check its connectivity and scenario output, and distinguish mock results from
live results in your pull request. A mock pass does not prove live recovery,
Redis cleanup, or PostgreSQL persistence.

## Change guidelines

- Keep changes focused and include regression tests for behavior changes.
- Format Go code with `gofmt`; follow existing Java and TypeScript conventions.
- Update `docs/contracts.md` when changing API payloads, Redis schemas, or lifecycle transitions.
- Describe the motivation, checks run, and any unverified behavior in the pull request.
- Never commit credentials, local environment files, generated builds, or private reference material.

## Reporting issues

Include reproduction steps, expected and actual behavior, relevant versions,
and sanitized logs. Do not post credentials or sensitive payloads. For a
security issue, use GitHub private vulnerability reporting if enabled rather
than opening a public issue with sensitive details.

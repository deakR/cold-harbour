# Contributing

## Prerequisites

- Go 1.25.1, as specified in `go.mod`
- Node.js 22 for `dashboard/`
- Docker with Compose
- Bash for `scripts/gen-certs.sh` on Windows

## Local setup

Set `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, and a valid `SIGNING_KEY`. Generate the local certificates, then follow the local setup steps in `README.md`:

```sh
bash scripts/gen-certs.sh
docker compose up -d postgres redis vault
docker compose up vault-init
```

The control plane and worker are run from Go during local development. The dashboard uses `npm install` and `npm run dev` from `dashboard/`.

## Tests and checks

Run the Go build, vet, and unit tests from the repository root:

```sh
go build ./...
go vet ./...
go test ./...
```

Run the realistic detector accuracy test:

```sh
go test ./internal/detect -run TestRealisticAccuracy -count=1
```

Run the detector load gate that CI runs:

```sh
go run ./cmd/sentinel-load
```

Run the dashboard checks from `dashboard/`:

```sh
npm ci
npm run build
npm run lint
```

The exact CI test sequence is:

```sh
go run ./cmd/coldharbour migrate
COLDHARBOUR_INTEGRATION=1 go test ./...
go run ./cmd/sentinel-load
WRITE_PROFILE=1 go run ./cmd/sentinel-load
cd dashboard
npm ci && npm run build
```

Tests that use Postgres or Redis need the same service environment as CI. Do not treat skipped integration tests as evidence that those services work.

## Branches and commits

Create branches from `main`. Use Conventional Commit subjects such as `fix: ...`, `docs: ...`, or `chore: ...`. Keep unrelated changes in separate pull requests.

## Pull requests

Run the checks above before opening a pull request. Describe the change and its verification. A pull request must have green CI before it is merged.

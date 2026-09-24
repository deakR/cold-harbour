# Contributing

## Prerequisites

- Go 1.25.1, as specified in `go.mod`
- Node.js 22 for `dashboard/`
- Docker with Compose
- Bash for `scripts/gen-certs.sh` on Windows

## Run from source

The [Quickstart](README.md#quickstart) runs everything in Docker. To work on the Go services, run only the backing services in Docker and run the control plane and worker from source.

1. Generate certificates and set `POSTGRES_PASSWORD`, `REDIS_PASSWORD`, and `SIGNING_KEY` as in the Quickstart.

2. Start Postgres, Redis, and Vault.

   ```powershell
   bash scripts/gen-certs.sh
   docker compose up -d postgres redis vault
   docker compose up vault-init
   ```

3. Set these variables in each terminal that runs a Go service.

   ```powershell
   $env:POSTGRES_DSN = "postgres://coldharbour:$env:POSTGRES_PASSWORD@127.0.0.1:5433/coldharbour?sslmode=verify-full&sslrootcert=docker/certs/ca.crt"
   $env:REDIS_ADDR = "127.0.0.1:6380"
   $env:REDIS_TLS = "1"
   $env:REDIS_CA = "docker/certs/ca.crt"
   $env:VAULT_ADDR = "http://127.0.0.1:8200"
   $env:VAULT_TOKEN = "coldharbour-dev"
   $env:ALLOW_INSECURE_SESSION_COOKIE = "1"
   $env:PORT = "8081"
   ```

4. Start the control plane in one terminal and a worker in another.

   ```powershell
   go run ./cmd/controlplane
   ```

   ```powershell
   go run ./cmd/worker -consumer worker-1
   ```

5. Create a tenant with `go run ./cmd/coldharbour admin create-tenant --name dev`. For the dashboard, run `npm install` and `npm run dev` in `dashboard/`.

Do not run `docker compose up` for the `controlplane` or `worker` services at the same time, because the control plane container also uses port `8081`.

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

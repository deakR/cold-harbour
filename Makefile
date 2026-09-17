.PHONY: build up stack verify verify-live bench logs down

build:
	cd services/worker-engine && go build ./...
	cd services/control-plane && ./mvnw -B -q -DskipTests package
	cd web && npm ci && npm run build

up:
	docker compose -f docker/docker-compose.yml up -d redis postgres

stack:
	docker compose -f docker/docker-compose.yml up -d --build

verify:
	powershell -ExecutionPolicy Bypass -File scripts/verify_e2e.ps1 -MockMode

verify-live:
	node scripts/verify_live.mjs

bench:
	powershell -ExecutionPolicy Bypass -File scripts/benchmark.ps1 -Count 50

logs:
	docker compose -f docker/docker-compose.yml logs -f

down:
	docker compose -f docker/docker-compose.yml down

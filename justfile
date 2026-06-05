# GoblinFTP task runner — https://just.systems
set dotenv-load

default:
    @just --list

# ── Development ────────────────────────────────────────────────────────────────

# Start frontend + backend together (requires overmind: https://github.com/DarthSim/overmind)
dev:
	overmind start -f Procfile

# Start frontend dev server only
dev-fe:
	cd frontend && pnpm run dev

# Start backend dev server only
dev-be:
	cd backend && go run ./cmd/gftp

# ── Build ──────────────────────────────────────────────────────────────────────

# Build everything
build: build-fe build-be

# Build Nuxt SPA → frontend/.output/public/
build-fe:
	cd frontend && pnpm run generate

# Build Go binary → bin/gftp
build-be:
	mkdir -p bin
	cd backend && go build -o ../bin/gftp ./cmd/gftp

# ── Test ───────────────────────────────────────────────────────────────────────

# Run all tests
test: test-fe test-be

# Run frontend tests
test-fe:
	cd frontend && pnpm test

# Run backend tests
test-be:
	cd backend && go test ./...

# ── Lint / Format ──────────────────────────────────────────────────────────────

# Run all linters
lint: lint-fe lint-be

# Lint + type-check frontend
lint-fe:
	cd frontend && pnpm run lint && pnpm run typecheck

# Lint backend (requires golangci-lint: https://golangci-lint.run)
lint-be:
	cd backend && golangci-lint run ./...

# Format all code
fmt:
	cd frontend && pnpm exec prettier --write .
	cd backend && gofmt -w .

# ── Docker ─────────────────────────────────────────────────────────────────────

# Build Docker image
docker-build:
	docker build -t darthsoup/goblinftp .

# Run Docker image
docker-run:
	docker run -p 8080:80 darthsoup/goblinftp

# Push Docker image
docker-push:
	docker push darthsoup/goblinftp

# Start with docker compose
docker-up:
	docker compose up --build

# Stop docker compose
docker-down:
	docker compose down

# Start FTP test server (garethflowers/ftp-server — default: ftpuser/ftppass on localhost:21)
ftp-up:
	docker compose --profile testing up ftp -d

# Stop FTP test server
ftp-down:
	docker compose --profile testing down ftp

# Start local S3 server for chunk staging (MinIO — default: minioadmin/minioadmin on localhost:9000, console on :9001)
s3-up:
	docker compose --profile testing up minio minio-init -d

# Stop local S3 server
s3-down:
	docker compose --profile testing down minio minio-init

# ── Utilities ──────────────────────────────────────────────────────────────────

# Report i18n keys in en.json missing from de.json
i18n-check:
	node -e "const en=require('./frontend/i18n/locales/en.json'),de=require('./frontend/i18n/locales/de.json'),m=Object.keys(en).filter(k=>de[k]===undefined);m.length?(console.log('Missing in de.json:',m),process.exit(1)):console.log('All keys present in de.json')"

# Remove build artifacts
clean:
	rm -rf frontend/.output frontend/node_modules bin/ frontend/.pnpm-store

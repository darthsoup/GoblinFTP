# GoblinFTP task runner — https://just.systems
set dotenv-load

default:
    @just --list

# Start frontend + backend together (concurrently — installed via `pnpm install`)
[group('dev')]
dev:
    pnpm exec concurrently -k -n backend,frontend -c blue,green "just dev-be" "just dev-fe"

# Start frontend dev server only (:3000)
[group('dev')]
dev-fe:
    cd frontend && pnpm run dev

# A relative GFTP_DATA_DIR (e.g. `data` from .env) resolves against the repo root —
# dev data (themes/known_hosts/staging) lives in <repo>/data, but `go run` runs
# from backend/, so a bare relative path would otherwise land in backend/data.
# Start backend dev server only (:8080)
[group('dev')]
dev-be:
    #!/usr/bin/env bash
    dir="${GFTP_DATA_DIR:-data}"
    [[ "$dir" = /* ]] || dir="{{ justfile_directory() }}/$dir"
    cd "{{ justfile_directory() }}/backend" && GFTP_DATA_DIR="$dir" go run ./cmd/gftp

# Build everything
[group('build')]
build: build-fe build-be

# Build Nuxt SPA → frontend/.output/public/
[group('build')]
build-fe:
    cd frontend && pnpm run generate

# Analyze the frontend bundle (treemap of chunk sizes)
[group('build')]
analyze-fe:
    cd frontend && pnpm run analyze

# Build Go binary → bin/gftp
[group('build')]
build-be:
    mkdir -p bin
    cd backend && go build -o ../bin/gftp ./cmd/gftp

# Run all tests
[group('test')]
test: test-fe test-be

# Run frontend tests (vitest)
[group('test')]
test-fe:
    cd frontend && pnpm test

# Run backend tests
[group('test')]
test-be:
    cd backend && go test ./...

# Run all linters
[group('lint')]
lint: lint-fe lint-be

# Lint + type-check frontend
[group('lint')]
lint-fe:
    cd frontend && pnpm run lint && pnpm run typecheck

# Lint backend (requires golangci-lint)
[group('lint')]
lint-be:
    cd backend && golangci-lint run ./...

# Format all code (eslint --fix + gofmt/goimports via golangci-lint)
[group('lint')]
fmt:
    cd frontend && pnpm run lint:fix
    cd backend && golangci-lint fmt

# Build Docker image
[group('docker')]
docker-build:
    docker build -t ghcr.io/darthsoup/goblinftp .

# Run Docker image (:8080)
[group('docker')]
docker-run:
    docker run -p 8080:80 ghcr.io/darthsoup/goblinftp

# Push Docker image (docker login ghcr.io first — releases are normally published by CI on v* tags)
[group('docker')]
docker-push:
    docker push ghcr.io/darthsoup/goblinftp

# Start dev stack (builds from source; docker-compose.override.yml auto-merges)
[group('docker')]
docker-up:
    docker compose up --build

# Stop dev stack
[group('docker')]
docker-down:
    docker compose down

# Start prod stack (pulls the published image; needs secrets in .env)
[group('docker')]
docker-up-prod:
    docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# Stop prod stack
[group('docker')]
docker-down-prod:
    docker compose -f docker-compose.yml -f docker-compose.prod.yml down

# Start local FTP test server (ftpuser/ftppass on :21)
[group('services')]
ftp-up:
    docker compose --profile testing up ftp -d

# Stop local FTP test server
[group('services')]
ftp-down:
    docker compose --profile testing down ftp

# Start local FTPS test server (explicit TLS, self-signed cert; ftpuser/ftppass on :2121, passive 30000-30009)
[group('services')]
ftps-up:
    docker compose --profile testing up ftps -d

# Stop local FTPS test server
[group('services')]
ftps-down:
    docker compose --profile testing down ftps

# Start local SFTP test server (ftpuser/ftppass on :2222, writable dir: /upload)
[group('services')]
sftp-up:
    docker compose --profile testing up sftp -d

# Stop local SFTP test server
[group('services')]
sftp-down:
    docker compose --profile testing down sftp

# Start local S3 server for chunk staging (minioadmin/minioadmin on :9000, console :9001)
[group('services')]
s3-up:
    docker compose --profile testing up minio minio-init -d

# Stop local S3 server
[group('services')]
s3-down:
    docker compose --profile testing down minio minio-init

# Generate a one-time SSO login link (see examples/sso/README.md)
[group('utils')]
sso-link *ARGS:
    cd backend && go run ./cmd/gftp-sso-link {{ ARGS }}

# Check every locale file under frontend/i18n/locales for full key + {…} placeholder parity with en.json
[group('utils')]
i18n-check:
    node frontend/scripts/i18n-check.mjs

# Regenerate .env.example and the doc config tables from the registry
[group('utils')]
confgen:
    cd backend && go run ./cmd/gftp-confgen -root "{{ justfile_directory() }}"

# Remove build artifacts
[group('utils')]
clean:
    rm -rf frontend/.output frontend/.nuxt node_modules frontend/node_modules bin/

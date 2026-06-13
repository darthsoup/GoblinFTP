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

# Start backend dev server only (:8080)
[group('dev')]
dev-be:
    cd backend && go run ./cmd/gftp

# Build everything
[group('build')]
build: build-fe build-be

# Build Nuxt SPA → frontend/.output/public/
[group('build')]
build-fe:
    cd frontend && pnpm run generate

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

# Start with docker compose
[group('docker')]
docker-up:
    docker compose up --build

# Stop docker compose
[group('docker')]
docker-down:
    docker compose down

# Start local FTP test server (ftpuser/ftppass on :21)
[group('services')]
ftp-up:
    docker compose --profile testing up ftp -d

# Stop local FTP test server
[group('services')]
ftp-down:
    docker compose --profile testing down ftp

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

# Report i18n keys in en.json missing from de.json
[group('utils')]
i18n-check:
    node -e "const en=require('./frontend/i18n/locales/en.json'),de=require('./frontend/i18n/locales/de.json'),m=Object.keys(en).filter(k=>de[k]===undefined);m.length?(console.log('Missing in de.json:',m),process.exit(1)):console.log('All keys present in de.json')"

# Remove build artifacts
[group('utils')]
clean:
    rm -rf frontend/.output frontend/.nuxt node_modules frontend/node_modules bin/

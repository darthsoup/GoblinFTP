# Development

**Requirements:** Go 1.26+, Node 24, [pnpm](https://pnpm.io), Docker, [just](https://just.systems)

```bash
cp .env.example .env   # if available, or create .env
just dev               # start frontend (port 3000) + backend (port 8080) with hot reload
```

## Common commands

```bash
just dev-fe       # frontend only
just dev-be       # backend only

just test         # run all tests
just test-fe      # Vitest (frontend)
just test-be      # Go tests (backend)

just build        # build frontend SPA + Go binary
just docker-build # build Docker image
just docker-up    # start via docker compose

just lint         # eslint + nuxt typecheck + golangci-lint
just fmt          # eslint --fix + gofmt
just i18n-check   # verify German translations are complete

just ftp-up       # start local FTP test server (garethflowers/ftp-server)
just ftp-down     # stop local FTP test server
just s3-up        # start local S3 server for chunk staging (MinIO)
just s3-down      # stop local S3 server
```

## Testing with a local FTP server

```bash
just ftp-up
# Connect with: localhost:21, ftpuser / ftppass
just ftp-down
```

## Testing with a local S3 server (MinIO)

```bash
just s3-up   # MinIO on localhost:9000 (console: localhost:9001, minioadmin/minioadmin)
             # the gftp-chunks bucket is created automatically

GFTP_S3_ENABLED=true GFTP_S3_ENDPOINT=http://localhost:9000 \
GFTP_S3_BUCKET=gftp-chunks GFTP_S3_ACCESS_KEY=minioadmin \
GFTP_S3_SECRET_KEY=minioadmin just dev-be

# S3 integration tests:
cd backend && GFTP_TEST_S3_ENDPOINT=http://localhost:9000 go test ./internal/staging/...

just s3-down
```

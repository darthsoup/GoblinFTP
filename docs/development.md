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
just i18n-check   # verify every locale file matches en.json (keys + placeholders)

just ftp-up       # start local FTP test server (garethflowers/ftp-server)
just ftp-down     # stop local FTP test server
just ftps-up      # start local FTPS test server (bfren/ftps, explicit TLS)
just ftps-down    # stop local FTPS test server
just sftp-up      # start local SFTP test server (jmcombs/sftp)
just sftp-down    # stop local SFTP test server
just s3-up        # start local S3 server for chunk staging (MinIO)
just s3-down      # stop local S3 server
```

## Testing with a local FTP server

```bash
just ftp-up
# Connect with: localhost:21, ftpuser / ftppass

# FTP integration tests:
cd backend && GFTP_TEST_FTP_HOST=localhost:21 go test ./internal/ftp/...

just ftp-down
```

## Testing with a local FTPS server

```bash
just ftps-up
# Connect with: localhost:2121, ftpuser / ftppass, protocol "ftps"
# The cert is self-signed — set GFTP_CONNECTION_FTP_TLS_INSECURE_SKIP_VERIFY=true when testing via the app.

# FTPS integration tests:
cd backend && GFTP_TEST_FTPS_HOST=localhost:2121 go test ./internal/ftp/...

just ftps-down
```

## Testing with a local SFTP server

```bash
just sftp-up
# Connect with: localhost:2222, ftpuser / ftppass, protocol "sftp"
# The user is chrooted: / is read-only — upload files under /upload.

# SFTP integration tests:
cd backend && GFTP_TEST_SFTP_HOST=localhost:2222 go test ./internal/sftp/...

just sftp-down
```

SSH host keys persist in the `sftp-ssh` volume, so the fingerprint survives
`just sftp-down` + `just sftp-up`. If you reset that volume
(`docker volume rm goblinftp_sftp-ssh`), the next start generates new keys and
a previously trusted connection fails with a host-key mismatch — delete the
`[localhost]:2222` line from `$GFTP_DATA_DIR/known_hosts` (dev:
`data/known_hosts` in the repo root, container: `/app/data/known_hosts`) and
re-trust.

The backend needs a writable `GFTP_DATA_DIR` for SFTP host-key pinning and
local upload staging. `just dev-be` defaults it to `data` and resolves a
relative value against the repo root, so the dev data dir is `<repo>/data`. If
your `.env` still sets the container path `/app/data`, change it to `data` (see
`.env.example`).

## Testing with a local S3 server (MinIO)

```bash
just s3-up   # MinIO on localhost:9000 (console: localhost:9001, minioadmin/minioadmin)
             # the gftp-chunks bucket is created automatically
             # (override the test server's root creds with MINIO_ROOT_USER /
             #  MINIO_ROOT_PASSWORD / MINIO_TEST_BUCKET; they are deliberately
             #  separate from the app-side GFTP_S3_* variables)

GFTP_S3_ENABLED=true GFTP_S3_ENDPOINT=http://localhost:9000 \
GFTP_S3_BUCKET=gftp-chunks GFTP_S3_ACCESS_KEY=minioadmin \
GFTP_S3_SECRET_KEY=minioadmin just dev-be

# S3 integration tests:
cd backend && GFTP_TEST_S3_ENDPOINT=http://localhost:9000 go test ./internal/staging/...

just s3-down
```

## See also

- [Installation](installation.md) for Docker and manual (from-source) setup.
- [Configuration](configuration.md) for the environment-variable reference.
- [Translations (i18n)](i18n.md) for adding or improving a language.


---
description: Run GoblinFTP locally, test your changes, and edit the docs.
---

# Development

Install Go 1.26 or newer, Node 24, pnpm, and [just](https://just.systems). You also need Docker to run the test servers. See [System requirements](system-requirements.md#building-from-source) for the tool versions.

From the repository root:

```bash
pnpm install
cp .env.example .env
just dev
```

Open `http://localhost:3000`. The backend runs on port 8080.

## Common commands

| Task | Command |
|---|---|
| Start only the frontend or backend | `just dev-fe` or `just dev-be` |
| Run all tests | `just test` |
| Run frontend or backend tests | `just test-fe` or `just test-be` |
| Build the app | `just build` |
| Format code | `just fmt` |
| Run lint and type checks | `just lint` |
| Check translations | `just i18n-check` |
| Build the Docker image | `just docker-build` |
| Start the development Docker setup | `just docker-up` |

Run `just --list` to see all commands.

## Documentation site

Edit the Markdown files in `docs/`. The site uses VitePress 2 and publishes changes from `main` to [GitHub Pages](https://darthsoup.github.io/GoblinFTP/).

```bash
just docs-dev
```

Open `http://localhost:5173/GoblinFTP/`. Changes appear as you save. The docs do not need the app backend.

Before committing, build the site to check for broken page links:

```bash
just docs-build
```

Use `just docs-preview` to view the built site at `http://localhost:4173/GoblinFTP/`. The equivalent pnpm commands are `pnpm docs:dev`, `pnpm docs:build`, and `pnpm docs:preview`.

Link to other guides with relative paths such as `configuration.md`. Use full GitHub URLs for files outside `docs/`. Add new guides to the sidebar in `.vitepress/config.ts` and give each page a description in frontmatter.

Settings tables are generated from `backend/internal/config/registry.go`. Edit descriptions there, then run `just confgen`. Check the generated files without changing them:

```bash
go -C backend test ./internal/config/gen
```

::: details GitHub Pages setup

In repository settings, choose **Pages > Build and deployment > Source: GitHub Actions**. Pull requests build the docs without publishing them. Successful `main` builds publish the site at `/GoblinFTP/`. Manual workflow runs publish only when run against `main`.

:::

## Test the Docker container

```bash
just docker-test
```

This needs Python 3 and Docker. It tests startup, routing, headers, proxy handling, shutdown, S3 startup, and data-folder permissions. It creates temporary containers, a network, and a volume, then removes them. CI and release checks run the same tests.

## Testing with a local FTP server

```bash
just ftp-up
GFTP_TEST_FTP_HOST=localhost:21 go -C backend test ./internal/ftp/...
just ftp-down
```

Connect with `localhost:21`, username `ftpuser`, and password `ftppass`.

## Testing with a local FTPS server

```bash
just ftps-up
GFTP_TEST_FTPS_HOST=localhost:2121 go -C backend test ./internal/ftp/...
just ftps-down
```

Connect with `localhost:2121`, username `ftpuser`, password `ftppass`, and protocol `ftps`.

The test certificate is self-signed. When connecting through the app, set `GFTP_CONNECTION_FTP_TLS_INSECURE_SKIP_VERIFY=true` for this local test.

## Testing with a local SFTP server

```bash
just sftp-up
GFTP_TEST_SFTP_HOST=localhost:2222 go -C backend test ./internal/sftp/...
just sftp-down
```

Connect with `localhost:2222`, username `ftpuser`, and password `ftppass`. Upload into `/upload`; the root folder is read-only.

Server keys are saved in the `sftp-ssh` Docker volume. If you intentionally delete that volume, the server gets a new key. Remove the old `[localhost]:2222` entry from `data/known_hosts` before trusting the new one.

The backend needs a writable data folder to save SFTP keys and temporary uploads. `just dev-be` uses `data/` in the repository root by default. If your `.env` sets the container path `/app/data`, change it to `data` for local development.

## Testing with a local S3 server (MinIO)

```bash
just s3-up
```

MinIO runs on `http://localhost:9000`, with its console on port 9001. The username and password are both `minioadmin`. The setup creates a `gftp-chunks` bucket.

Start the backend with S3 storage enabled:

```bash
GFTP_S3_ENABLED=true GFTP_S3_ENDPOINT=http://localhost:9000 \
GFTP_S3_BUCKET=gftp-chunks GFTP_S3_ACCESS_KEY=minioadmin \
GFTP_S3_SECRET_KEY=minioadmin just dev-be
```

Run the tests in another terminal, then stop MinIO when finished:

```bash
GFTP_TEST_S3_ENDPOINT=http://localhost:9000 go -C backend test ./internal/staging/...
just s3-down
```

To change the test server's login or bucket, set `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD`, or `MINIO_TEST_BUCKET`. These are separate from GoblinFTP's `GFTP_S3_*` settings.

## See also

- [Installation](installation.md) to run GoblinFTP outside development.
- [Configuration](configuration.md) for app settings.
- [Translations](i18n.md) to improve or add a language.

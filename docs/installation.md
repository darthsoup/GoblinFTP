# Installation

GoblinFTP ships as a single multi-arch image: a static Go binary plus Caddy on `caddy:2-alpine`. Inside the container the entrypoint starts the backend, polls `http://localhost:8080/healthz` for up to 5 seconds (10 tries at 0.5s), then starts Caddy on port 80. If the backend exits before it is healthy, or either process later dies, the container exits non-zero. The image healthcheck polls `/healthz` through Caddy on port 80, so it covers the full serving chain. Docker is the recommended deployment; a from-source install behind your own web server is also supported.

Check the [system requirements](system-requirements.md) first.

## Docker

### Quick start

```bash
docker run -p 8080:80 ghcr.io/darthsoup/goblinftp
```

Open <http://localhost:8080>, enter FTP/SFTP credentials, and connect. Caddy serves on port 80 inside the container; map it to any host port (`8080:80` above).

### Production run

Set the signing secrets and mount a persistent `/app/data` volume. Without the secrets, ephemeral 32-byte keys are generated on each start, invalidating all sessions and download links on restart.

```bash
docker run -d --name goblinftp \
  -p 8080:80 \
  -e GFTP_SESSION_SECRET="$(openssl rand -hex 32)" \
  -e GFTP_DOWNLOAD_TOKEN_SECRET="$(openssl rand -hex 32)" \
  -v goblinftp-data:/app/data \
  --restart unless-stopped \
  ghcr.io/darthsoup/goblinftp
```

`/app/data` holds:

- `known_hosts` for SFTP host-key pinning (keep it persistent so trusted keys survive restarts).
- Local upload staging (unless [S3 chunk staging](s3-staging.md) is enabled).
- Per-tenant [theme](theming.md) assets under `themes/<tenant>/`.
UI, editor, connection, and access defaults are all environment variables too. For more than a handful, use an env file (start from [`.env.example`](../.env.example)):

```bash
docker run -d --name goblinftp \
  -p 8080:80 \
  --env-file .env \
  -v goblinftp-data:/app/data \
  --restart unless-stopped \
  ghcr.io/darthsoup/goblinftp
```

Every variable is documented in [Configuration](configuration.md).

### Docker Compose

The repository splits Compose into a shared base and two overlays, following the standard Compose merge convention:

| File | Role |
|---|---|
| [`docker-compose.yml`](../docker-compose.yml) | Base. Common `gftp` service (image, ports, `env_file`, restart). Loads a local `.env` into the container when present (optional `env_file`, needs Compose v2.24+), so every `GFTP_*` variable set there reaches GoblinFTP. |
| [`docker-compose.override.yml`](../docker-compose.override.yml) | Development. Builds from source and adds local FTP/FTPS/SFTP/S3 test servers (`testing` profile). Auto-merged by a plain `docker compose` command. |
| [`docker-compose.prod.yml`](../docker-compose.prod.yml) | Production. Pulls the published image (no build), requires the signing secrets, and persists `/app/data`. Loaded explicitly with `-f`. |

**Development** (builds locally; the override merges automatically):

```bash
docker compose up --build        # or: just docker-up
```

Because the override is auto-loaded only when no `-f` flags are passed, its build step and test servers never leak into production.

**Production** (pulls the image; the override is not loaded):

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d   # or: just docker-up-prod
```

The prod overlay uses `${VAR:?}` interpolation for `GFTP_SESSION_SECRET` and `GFTP_DOWNLOAD_TOKEN_SECRET`, so `docker compose` fails fast if they are unset. Put them (and an optional `GFTP_VERSION` to pin the image tag) in a local `.env` file: Compose reads it for interpolation, and the base file passes it into the container via `env_file`, so any other `GFTP_*` variable you set there applies too. Note that `.env` is the interface: variables exported only in your shell no longer reach the container.

```bash
# .env (gitignored)
GFTP_VERSION=1.2.3
GFTP_SESSION_SECRET=...            # openssl rand -hex 32
GFTP_DOWNLOAD_TOKEN_SECRET=...     # openssl rand -hex 32
```

### Image tags

Published to GHCR on every `v*` tag, all multi-arch (`linux/amd64`, `linux/arm64`):

- `:1.2.3` for an exact release. Pin this in production.
- `:1.2` / `:1` for the latest patch or minor of a line.
- `:latest` for the latest release. Prerelease tags (`v1.0.0-rc1`) are excluded from `:latest`.
- `:main` for the current `main`, unreleased (reports version `dev`).

The version is injected at build time (`VERSION` build-arg into `main.version`) and surfaces in the startup log, at `GET /healthz`, in `/api/system/vars`, and as the default Sentry release.

## Build the image yourself

```bash
git clone https://github.com/darthsoup/goblinftp.git
cd goblinftp
docker build -f docker/Dockerfile -t goblinftp .
docker run -p 8080:80 goblinftp
```

The multi-stage build compiles the Nuxt SPA (`node:24-alpine`) and cross-compiles the Go binary for the target arch (`golang:1.26-alpine`, `CGO_ENABLED=0`) on the build platform, so no Node, pnpm, or Go toolchain is needed on the host, only Docker. Pass `--build-arg VERSION=v1.2.3` to stamp a version.

## Manual install (from source)

Run without Docker to place GoblinFTP behind an existing web server. Install the [build toolchain](system-requirements.md#building-from-source) first (Go, Node, pnpm).

**1. Build the frontend SPA.**

```bash
pnpm install --frozen-lockfile
pnpm --filter goblinftp-frontend run generate
# Static output: frontend/.output/public/
```

**2. Build the backend binary.**

```bash
cd backend && go build -o gftp ./cmd/gftp
# Or from the repo root: `just build` writes it to bin/gftp
```

**3. Run the backend.**

The binary serves the API surface only (`/api/*`, `/healthz`, `/themes/*`, and the `/?sso=` entry). It does not serve the SPA. Point `GFTP_DATA_DIR` at a writable directory; otherwise SFTP connects fail trying to create the default `/app/data`.

```bash
GFTP_DATA_DIR=/var/lib/goblinftp \
GFTP_SESSION_SECRET="$(openssl rand -hex 32)" \
GFTP_DOWNLOAD_TOKEN_SECRET="$(openssl rand -hex 32)" \
./gftp   # listens on :8080 (GFTP_PORT)
```

**4. Front it with a web server.**

The proxy serves the static SPA and forwards backend routes. Replicate the container's routing (reference: [`docker/Caddyfile`](../docker/Caddyfile)):

- `/api/*`, `/healthz`, `/themes/*`, and `/` carrying an `sso=*` query go to the backend on `:8080`.
- Everything else serves `frontend/.output/public` with an SPA history fallback to `index.html`.

```caddy
:80 {
	@sso {
		path /
		query sso=*
	}
	handle /api/*    { reverse_proxy localhost:8080 }
	handle /healthz  { reverse_proxy localhost:8080 }
	handle /themes/* { reverse_proxy localhost:8080 }
	handle @sso      { reverse_proxy localhost:8080 }
	handle {
		root * /path/to/frontend/.output/public
		try_files {path} /index.html
		file_server
	}
}
```

Terminate HTTPS at this proxy. GoblinFTP transmits credentials, so serve it over TLS in production.

## Next steps

- [Configuration](configuration.md): the environment-variable reference.
- [Theming](theming.md): white-label branding and per-tenant themes.
- [SSO login links](../examples/sso/README.md): one-time direct-login links.

# GoblinFTP

A self-hosted, web-based FTP/SFTP client. Deploy as a Docker container and manage remote files via browser.

Clean rewrite of [Monsta FTP](https://www.monstaftp.com/) v2.14.x with full feature parity and no licence gating.

## Stack

- **Backend:** Go + Echo
- **Frontend:** Nuxt 4 (SPA) · Nuxt UI v4 · Tailwind CSS v4
- **Container:** Docker (Caddy + Go binary)

## Quick start

```bash
docker run -p 8080:80 darthsoup/goblinftp
```

Open http://localhost:8080

## Configuration

Configuration is via environment variables:

| Variable | Default | Description |
|---|---|---|
| `GFTP_PAGE_TITLE` | `GoblinFTP` | Browser tab title |
| `GFTP_SESSION_SECRET` | _(auto-generated)_ | Session signing key |
| `GFTP_DOWNLOAD_TOKEN_SECRET` | _(auto-generated)_ | Download token signing key |
| `GFTP_SSO_ENABLED` | `false` | Enable SSO login links |
| `GFTP_SSO_SECRET` | — | Shared secret for SSO token validation |
| `GFTP_SENTRY_DSN` | — | Sentry DSN for error tracking |
| `GFTP_SETTINGS_PATH` | `/app/data/settings.json` | Path to settings file |

Mount a `settings.json` for UI/connection/access settings (see `settings.example.json`).

```bash
docker run -p 8080:80 \
  -e GFTP_PAGE_TITLE="My FTP Client" \
  -e GFTP_SSO_SECRET="your-shared-secret" \
  -v ./settings.json:/app/data/settings.json:ro \
  darthsoup/goblinftp
```

## Development

**Requirements:** Go 1.26+, Node 24, pnpm, Docker, [just](https://just.systems), [overmind](https://github.com/DarthSim/overmind)

```bash
just dev          # start frontend + backend (hot reload)
just dev-fe       # frontend only
just dev-be       # backend only

just test         # run all tests
just test-fe      # frontend tests
just test-be      # backend tests (Go)

just build        # build frontend SPA + Go binary
just docker-build # build Docker image
just docker-up    # start via docker compose

just lint         # typecheck + golangci-lint
just fmt          # prettier + gofmt
just i18n-check   # check for missing German translations
```

## Status

| Phase | Status | Description |
|---|---|---|
| 1 — Scaffold | ✅ Done | Repo structure, justfile, Dockerfile, CI |
| 2 — Backend core | ✅ Done | Config, logging, session/CSRF/auth middleware, Echo router |
| 3 — FTP/SFTP layer | 🔜 Next | Connect, file operations, download tokens |
| 4 — SSO & security | ⬜ Planned | SSO token validation, download HMAC tokens |
| 5 — Frontend | ⬜ Planned | Nuxt SPA: auth, file browser, transfer UI, editor |
| 6 — Polish | ⬜ Planned | German i18n, Sentry, test coverage ≥ 70% |

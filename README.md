# GoblinFTP

A self-hosted, web-based FTP/SFTP client. Deploy as a single Docker container and manage remote files from any browser — no license keys, no phone-home.

## Features

- **FTP & SFTP** support with passive mode
- **File browser** — upload, download, rename, delete, chmod, zip download
- **Text editor** with syntax highlighting (CodeMirror)
- **Drag-and-drop upload** with chunked transfer and progress panel
- **SSO** — generate signed login links for direct authentication
- **i18n** — English and German
- **Error tracking** via Sentry (optional)

## Stack

- **Backend:** Go + Echo v4
- **Frontend:** Nuxt 4 (SPA) · Nuxt UI v4 · Tailwind CSS v4 · Pinia
- **Container:** Docker (Caddy + Go binary)

## Quick start

```bash
docker run -p 8080:80 darthsoup/goblinftp
```

Open <http://localhost:8080>, enter your FTP/SFTP credentials and connect.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `GFTP_PAGE_TITLE` | `GoblinFTP` | Browser tab title |
| `GFTP_SESSION_SECRET` | _(auto-generated)_ | Session signing key — set this in production |
| `GFTP_DOWNLOAD_TOKEN_SECRET` | _(auto-generated)_ | Download token signing key — set this in production |
| `GFTP_SSO_ENABLED` | `false` | Enable SSO login links |
| `GFTP_SSO_SECRET` | — | Shared secret for SSO token validation |
| `GFTP_SENTRY_DSN` | — | Sentry DSN for backend error tracking |
| `GFTP_SETTINGS_PATH` | `/app/data/settings.json` | Path to settings file |
| `NUXT_PUBLIC_SENTRY_DSN` | — | Sentry DSN for frontend error tracking |

Mount a `settings.json` for UI/connection/access settings (see `settings.example.json`).

```bash
docker run -p 8080:80 \
  -e GFTP_PAGE_TITLE="My FTP Client" \
  -e GFTP_SESSION_SECRET="change-me" \
  -e GFTP_DOWNLOAD_TOKEN_SECRET="change-me" \
  -v ./settings.json:/app/data/settings.json:ro \
  darthsoup/goblinftp
```

### settings.json

Key options (full schema in `settings.example.json`):

| Setting | Description |
|---|---|
| `connection.allowedTypes` | Restrict to `["ftp"]`, `["sftp"]`, or both |
| `connection.disableChmod` | Hide chmod UI |
| `editor.disabled` | Disable the file editor entirely |
| `editor.allowedExtensions` | Restrict editable file extensions |
| `access.allowedClientAddresses` | IP allowlist (empty = allow all) |
| `ui.showDotFiles` | Show hidden files |

## Development

**Requirements:** Go 1.26+, Node 24, [pnpm](https://pnpm.io), Docker, [just](https://just.systems), [overmind](https://github.com/DarthSim/overmind)

```bash
cp .env.example .env   # if available, or create .env
just dev               # start frontend (port 3000) + backend (port 8080) with hot reload
```

### Common commands

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
just fmt          # prettier + gofmt
just i18n-check   # verify German translations are complete

just ftp-up       # start local FTP test server (garethflowers/ftp-server)
just ftp-down     # stop local FTP test server
```

### Testing with a local FTP server

```bash
just ftp-up
# Connect with: localhost:21, ftpuser / ftppass
just ftp-down
```

## License

MIT

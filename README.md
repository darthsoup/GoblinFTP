# GoblinFTP

A self-hosted, web-based FTP/SFTP client. Deploy as a single Docker container and manage remote files from any browser.

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
docker run -p 8080:80 ghcr.io/darthsoup/goblinftp
```

Open <http://localhost:8080>, enter your FTP/SFTP credentials and connect.

## Releases / image tags

Images are published to GHCR on every `v*` tag — all multi-arch (`linux/amd64`, `linux/arm64`):

- `ghcr.io/darthsoup/goblinftp:1.2.3` — exact release (pin this in production)
- `ghcr.io/darthsoup/goblinftp:1.2` / `:1` — latest patch / latest minor of a line
- `ghcr.io/darthsoup/goblinftp:latest` — latest release (exists once the first version is tagged)
- `ghcr.io/darthsoup/goblinftp:main` — current `main`, unreleased (reports version `dev`)

The running version shows up in the startup log, `GET /healthz`, and the settings dialog.

## Documentation

- **[Configuration](docs/configuration.md)** — environment variables, `settings.json`, logging, metrics, S3 chunk staging, and SSO login links.
- **[Development](docs/development.md)** — local setup, common `just` commands, and testing against a local FTP / S3 server.

## License

MIT

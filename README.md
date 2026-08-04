# GoblinFTP

A self-hosted, web-based FTP, FTPS, and SFTP client. Deploy as a single Docker container and manage remote files from any browser.

## Features

- **FTP, FTPS & SFTP** with passive mode and SFTP host-key pinning
- **File browser**: upload, download, rename, delete, chmod, and zip download
- **Text editor** with syntax highlighting (CodeMirror)
- **Drag-and-drop uploads** with chunked transfer and a progress panel
- **SSO**: signed, one-time login links for direct authentication
- **White-label branding** and per-tenant themes
- **i18n**: 13 languages, including English, German, French, Spanish, Italian, Dutch, Portuguese, Swedish, Danish, Norwegian, Finnish, Czech, and Slovak
- **Observability**: structured logging, optional Prometheus metrics, and optional Sentry error tracking

## Stack

- **Backend:** Go + Echo v4
- **Frontend:** Nuxt 4 (SPA), Nuxt UI v4, Tailwind CSS v4, Pinia
- **Container:** Docker (Caddy + Go binary)

## Quick start

```bash
docker run -p 8080:80 ghcr.io/darthsoup/goblinftp
```

Open <http://localhost:8080>, enter your FTP/SFTP credentials, and connect.

For a production setup (signing secrets, persistent storage, Docker Compose) and for building from source, see the [Installation guide](docs/installation.md). Check the [system requirements](docs/system-requirements.md) first.

## Releases

Images are published to GHCR on every `v*` tag, multi-arch (`linux/amd64`, `linux/arm64`). Pin an exact version (`:1.2.3`) in production. `:latest` tracks releases and `:main` tracks the unreleased `main` branch. See [image tags](docs/installation.md#image-tags) for the full list. The running version shows up in the startup log, at `GET /healthz`, and in the settings dialog.

## Documentation

- **[Installation](docs/installation.md)**: Docker, Docker Compose, and building from source.
- **[System requirements](docs/system-requirements.md)**: runtime footprint, networking, and build toolchain.
- **[Configuration](docs/configuration.md)**: environment variables and `settings.json`, with dedicated pages for [Logging](docs/logging.md), [Metrics](docs/metrics.md), and [S3 chunk staging](docs/s3-staging.md).
- **[Theming](docs/theming.md)**: white-label branding and per-tenant themes selected by SSO or subdomain.
- **[SSO login links](examples/sso/README.md)**: one-time direct-login links, with generators in Go, Node.js, and PHP.
- **[Iframe embedding](docs/embedding.md)**: dropping GoblinFTP into a hosting control panel as a pre-authenticated frame.
- **[Development](docs/development.md)**: local setup, `just` commands, and testing against local FTP / SFTP / S3 servers.
- **[Translations (i18n)](docs/i18n.md)**: how to add or improve a language.

## Contributing

Contributions are welcome. See **[Development](docs/development.md)** for local setup. Before opening a PR:

- `just fmt`: format frontend (eslint) and backend (gofmt)
- `just lint`: eslint, Nuxt typecheck, golangci-lint
- `just test`: backend (Go) and frontend (vitest) suites

**Adding or improving a translation?** See **[Translations (i18n)](docs/i18n.md)**.

## License

MIT

# GoblinFTP

A self-hosted, web-based FTP, FTPS, and SFTP client. Deploy as a single Docker container and manage remote files from any browser.

**[Documentation](https://darthsoup.github.io/GoblinFTP/)** · [Installation](https://darthsoup.github.io/GoblinFTP/installation.html) · [Configuration](https://darthsoup.github.io/GoblinFTP/configuration.html)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/screenshot-dark.png">
  <img alt="GoblinFTP" src="docs/assets/screenshot-light.png" width="600">
</picture>

## Features

- **FTP, FTPS & SFTP** with passive mode and SFTP host-key pinning
- **File browser**: upload, download, rename, delete, chmod, and zip download
- **Text editor** with syntax highlighting (CodeMirror)
- **Drag-and-drop uploads** with upload progress
- **SSO**: one-time links that sign users in from your app
- **Your logo and colors**, with a different theme for each customer if needed
- **13 languages**, including English, German, French, Spanish, Italian, Dutch, Portuguese, Swedish, Danish, Norwegian, Finnish, Czech, and Slovak
- **Monitoring**: logs, optional Prometheus metrics, and optional Sentry error reports

## Stack

- **Backend:** Go + Echo v4
- **Frontend:** Nuxt 4 (SPA), Nuxt UI v4, Tailwind CSS v4, Pinia
- **Container:** Docker (Caddy + Go binary)

## Quick start

```bash
docker run -p 8080:8080 ghcr.io/darthsoup/goblinftp
```

Open <http://localhost:8080>, enter your FTP/SFTP credentials, and connect.

For production deployment, persistent storage, Docker Compose, and building from source, see the [Installation guide](https://darthsoup.github.io/GoblinFTP/installation.html). Check the [system requirements](https://darthsoup.github.io/GoblinFTP/system-requirements.html) first.

## Releases

Images are published to GHCR on every `v*` tag, multi-arch (`linux/amd64`, `linux/arm64`). Pin an exact version (`:1.2.3`) in production. `:latest` tracks releases and `:main` tracks the unreleased `main` branch. See [image tags](https://darthsoup.github.io/GoblinFTP/installation.html#image-tags) for the full list. The running version shows up in the startup log, at `GET /healthz`, and in the settings dialog.

## Documentation

Read the **[full documentation](https://darthsoup.github.io/GoblinFTP/)**.

- **[Installation](https://darthsoup.github.io/GoblinFTP/installation.html)**: Docker, Docker Compose, and building from source.
- **[System requirements](https://darthsoup.github.io/GoblinFTP/system-requirements.html)**: runtime footprint, networking, and build toolchain.
- **[Configuration](https://darthsoup.github.io/GoblinFTP/configuration.html)**: every environment variable, with dedicated pages for [Logging](https://darthsoup.github.io/GoblinFTP/logging.html), [Metrics](https://darthsoup.github.io/GoblinFTP/metrics.html), [Error tracking](https://darthsoup.github.io/GoblinFTP/sentry.html), and [S3 upload storage](https://darthsoup.github.io/GoblinFTP/s3-staging.html).
- **[Branding and themes](https://darthsoup.github.io/GoblinFTP/theming.html)**: add your logo and colors, or a theme for each customer.
- **[SSO login links](https://darthsoup.github.io/GoblinFTP/sso.html)**: create one-time links with Go, Node.js, or PHP.
- **[Iframe embedding](https://darthsoup.github.io/GoblinFTP/embedding.html)**: show GoblinFTP inside your app and sign users in automatically.
- **[Development](https://darthsoup.github.io/GoblinFTP/development.html)**: local setup, `just` commands, and testing against local FTP / SFTP / S3 servers.
- **[Translations (i18n)](https://darthsoup.github.io/GoblinFTP/i18n.html)**: how to add or improve a language.

## Contributing

Contributions are welcome. See **[Development](https://darthsoup.github.io/GoblinFTP/development.html)** for local setup. Before opening a PR:

- `just fmt`: format frontend (eslint) and backend (gofmt)
- `just lint`: eslint, Nuxt typecheck, golangci-lint
- `just test`: backend (Go) and frontend (vitest) suites
- `just docs-build`: build the documentation and check internal page links

**Adding or improving a translation?** See **[Translations (i18n)](https://darthsoup.github.io/GoblinFTP/i18n.html)**.

## License

MIT

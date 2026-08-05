# System requirements

The image is a static `CGO_ENABLED=0` Go binary plus Caddy on `caddy:2-alpine`. GoblinFTP is a client to your existing FTP/SFTP servers and holds little state of its own, so the runtime footprint is small.

## Running with Docker (recommended)

| Requirement | Notes |
|---|---|
| Docker | Any currently supported Docker Engine. Compose is optional. |
| CPU | 1 core suffices for typical use. |
| Memory | Small and roughly flat. In-flight upload chunks are staged to disk or S3, not held in RAM, so memory does not scale with file size. A 128 MB container is comfortable for light use. |
| Disk | The image is small. Size the `/app/data` volume for local upload staging (below). |
| Architecture | `linux/amd64` and `linux/arm64` are both published. |

### Storage

Mount a persistent volume at `/app/data`. It holds:

- `known_hosts` for SFTP host-key pinning, so trusted keys survive restarts.
- Local upload staging: chunks buffered on disk before transfer, roughly one file's worth of chunks per concurrent upload, deleted after commit. Move this off local disk with [S3 chunk staging](s3-staging.md).
- Per-tenant [theme](theming.md) assets.

### Networking

- **Inbound:** only the host port you map to the container's port 80 (for example `8080:80`). Serve it over HTTPS in production, typically by terminating TLS at a reverse proxy in front of GoblinFTP.
- **Outbound:** to the FTP/SFTP servers users log in to, and to the S3 endpoint if S3 staging is enabled.

GoblinFTP is the FTP client, not a server. Passive-mode data connections are opened outbound to the remote server, so no inbound passive port range is required on GoblinFTP itself.

## Browser

A current evergreen browser (recent Chrome, Edge, Firefox, or Safari) with JavaScript enabled. The interface is a Nuxt 4 single-page app served as static ES-module output.

## Building from source

Required only for a [manual install](installation.md#manual-install-from-source) or [development](development.md). Building the Docker image needs none of these on the host, only Docker (the toolchains run inside build stages).

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.26 or newer (repo builds on 1.26.x) | Backend binary. |
| Node.js | 24 (see `.nvmrc`) | Nuxt SPA build. |
| pnpm | 11.x (pinned via `packageManager`; enable with `corepack enable`) | Frontend package manager. |
| [just](https://just.systems) | any | Optional. Runs the project task recipes. |

## See also

- [Installation](installation.md)
- [Configuration](configuration.md)
- [Development](development.md)

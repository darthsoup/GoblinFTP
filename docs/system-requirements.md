---
description: Check what you need to run GoblinFTP with Docker or build it from source.
---

# System requirements

You need Docker, a web browser, and an FTP, FTPS, or SFTP server to connect to.

## Running with Docker (recommended)

| Requirement | Recommendation |
|---|---|
| Docker | Docker with Linux containers. Docker Compose is optional. |
| CPU | 1 core for typical use. |
| Memory | Start with 128 MB for light use; allow more for heavier workloads. |
| Storage | Enough free space for files being uploaded. See below. |
| Processor | 64-bit Intel/AMD or ARM64. |

### Storage

Keep a Docker volume at `/app/data` so trusted SFTP keys and [custom themes](theming.md) survive restarts. Uploads also use this folder temporarily.

Allow enough free space for each file being uploaded at the same time. You can use [S3 storage](s3-staging.md) for temporary uploads instead.

### Networking

- Users need access to the port you publish, such as `8080`. For public access, put a web server in front of GoblinFTP that handles HTTPS.
- GoblinFTP needs outgoing access to your FTP, FTPS, or SFTP servers, and to S3 if enabled.

With passive FTP, you do not need to open an incoming FTP port range on the GoblinFTP host.

## Browser

A recent version of Chrome, Edge, Firefox, or Safari with JavaScript enabled.

## Building from source

You only need these tools for [manual installation](installation.md#manual-install-from-source) or [development](development.md). With Docker, they are installed for you.

| Tool | Version |
|---|---|
| Go | 1.26 or newer |
| Node.js | 24 |
| pnpm | The version pinned in `package.json`; enable with `corepack enable` |

[just](https://just.systems) is optional and provides shortcuts for common commands.

## See also

- [Installation](installation.md)
- [Configuration](configuration.md)
- [Development](development.md)

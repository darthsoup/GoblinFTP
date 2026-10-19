---
description: Get GoblinFTP running with Docker, use Docker Compose, or install it from source.
---

# Installation

Docker is the easiest way to run GoblinFTP. Check the [system requirements](system-requirements.md), then try the command below.

## Docker

### Quick start

```bash
docker run -p 8080:8080 ghcr.io/darthsoup/goblinftp
```

Open <http://localhost:8080>, enter your server address, username, and password, then connect.

### Production run

To keep GoblinFTP running after you close the terminal, use:

```bash
docker run -d --name goblinftp \
  -p 8080:8080 \
  -v goblinftp-data:/app/data \
  --restart unless-stopped \
  ghcr.io/darthsoup/goblinftp
```

The `-v` option saves trusted SFTP keys and custom themes in a Docker volume. It also provides space for temporary uploads. Restarting GoblinFTP signs users out and makes their download links expire.

For public access, put a web server in front of GoblinFTP that handles HTTPS.

### Configuration

To change settings, create a file named `.env`. For example, this shows hidden files and sets German as the default language:

```bash
GFTP_UI_SHOW_DOT_FILES=true
GFTP_LANGUAGE=de
```

Add `--env-file .env` to your `docker run` command to load the file. See [Configuration](configuration.md) for all settings, or copy [`.env.example`](https://github.com/darthsoup/goblinftp/blob/main/.env.example) to get started.

### Docker Compose

Clone the repository and start the production setup:

```bash
git clone https://github.com/darthsoup/goblinftp.git
cd goblinftp
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

This downloads GoblinFTP, saves its data in a Docker volume, and loads your `.env` file if you have one. You need Docker Compose 2.24 or newer.

Set `GFTP_VERSION` in `.env` to choose a version. Keep both `-f` options when running this command. For working on GoblinFTP itself, see [Development](development.md).

### Image tags

- `:latest`: the latest stable release.
- `:main`: the current development build.
- A version such as `:1.2.3`: a specific release. Pin a version for production.

The running version is shown in the settings dialog.

## Build the image yourself

From the repository root:

```bash
docker build -f docker/Dockerfile -t goblinftp .
docker run -p 8080:8080 goblinftp
```

You only need Docker. It installs the build tools for you.

## Manual install (from source)

For a setup without Docker, install the [build tools](system-requirements.md#building-from-source) and clone the repository. From its root, build the application and start the backend:

```bash
pnpm install --frozen-lockfile
pnpm --filter goblinftp-frontend run generate
mkdir -p bin
go -C backend build -o ../bin/gftp ./cmd/gftp
GFTP_BIND_ADDRESS=127.0.0.1 GFTP_DATA_DIR=./data ./bin/gftp
```

The backend runs on port 8080. A web server must serve the interface from `frontend/.output/public` and send API requests to the backend. Here is an example using Caddy:

```caddy
files.example.com {
    @backend path /api/* /healthz /themes/*
    @sso {
        path /
        query sso=*
    }
    handle @backend {
        reverse_proxy localhost:8080
    }
    handle @sso {
        reverse_proxy localhost:8080
    }
    handle {
        root * /path/to/frontend/.output/public
        try_files {path} /index.html
        file_server
    }
}
```

Replace the domain and file path with your own. Caddy enables HTTPS for your domain automatically. For additional headers and iframe support, see the container's [Caddy configuration](https://github.com/darthsoup/goblinftp/blob/main/docker/Caddyfile).

## Next steps

- [Configuration](configuration.md): customize your setup.
- [Theming](theming.md): add your branding.
- [SSO login links](sso.md): connect users directly from another application.

For changed security defaults and SSO confirmation, see [Audit remediation and upgrade notes](audit-remediation.md).

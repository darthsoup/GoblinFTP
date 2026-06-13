# Configuration

| Variable | Default | Description |
|---|---|---|
| `GFTP_PAGE_TITLE` | `GoblinFTP` | Browser tab title |
| `GFTP_APP_NAME` | `GoblinFTP` | White-label app name (header, login, title, footer) |
| `GFTP_LOGO_URL` / `GFTP_FAVICON_URL` | — | White-label logo + favicon image URLs |
| `GFTP_PRIMARY_COLOR` | — | Accent color as hex (e.g. `#2563eb`) — recolors the theme at runtime |
| `GFTP_TAGLINE` | — | Login tagline override |
| `GFTP_HIDE_ATTRIBUTION` | `false` | Hide the app-name/version footer |
| `GFTP_SESSION_SECRET` | _(auto-generated)_ | Session signing key — set this in production |
| `GFTP_DOWNLOAD_TOKEN_SECRET` | _(auto-generated)_ | Download token signing key — set this in production |
| `GFTP_SSO_ENABLED` | `false` | Enable SSO login links |
| `GFTP_SSO_SECRET` | — | Shared secret for SSO token validation |
| `GFTP_SENTRY_DSN` | — | Sentry DSN for backend error tracking |
| `GFTP_SETTINGS_PATH` | `/app/data/settings.json` | Path to settings file |
| `NUXT_PUBLIC_SENTRY_DSN` | — | Sentry DSN for frontend error tracking |

Mount a `settings.json` for UI/connection/access settings (see [`settings.example.json`](../settings.example.json)).

```bash
docker run -p 8080:80 \
  -e GFTP_PAGE_TITLE="My FTP Client" \
  -e GFTP_SESSION_SECRET="change-me" \
  -e GFTP_DOWNLOAD_TOKEN_SECRET="change-me" \
  -v ./settings.json:/app/data/settings.json:ro \
  ghcr.io/darthsoup/goblinftp
```

## Logging

The backend writes structured logs to stdout — one line per request (method, path, status, duration, request ID, client IP, and the connected user/host once logged in) plus a `frontend error` line for browser-side errors forwarded by the SPA. Failed operations carry the machine-readable `error_code` and the underlying `cause`, so `docker logs` tells you *why* something failed without leaking raw socket errors to the browser.

| Variable | Default | Description |
|---|---|---|
| `GFTP_LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` — at `warn`, successful-request lines disappear but failures stay |
| `GFTP_LOG_FORMAT` | `json` | `json` (machine-readable) or `text` (human-friendly, nice for development) |
| `GFTP_LOG_FILE` | — | Additionally mirror logs into this file with size-based rotation (stdout is always written) |
| `GFTP_LOG_FILE_MAX_SIZE_MB` | `10` | Rotate the file after this size |
| `GFTP_LOG_FILE_MAX_BACKUPS` | `5` | Rotated files to keep |
| `GFTP_LOG_FILE_MAX_AGE_DAYS` | `0` | Delete rotated files older than this (`0` = keep regardless of age) |
| `GFTP_LOG_FRONTEND` | `true` | Accept browser-error reports on `POST /api/log/frontend` (rate-limited per IP, no auth required) |

```bash
# Docker-native: just read the container output (ship it with your log driver / Loki / ELK)
docker logs -f goblinftp

# Optional file sink on the data volume, e.g. for setups without a log collector
docker run -p 8080:80 \
  -e GFTP_LOG_FILE=/app/data/logs/gftp.log \
  -v gftp-data:/app/data \
  ghcr.io/darthsoup/goblinftp
```

Notes: the full session ID never appears in logs (only an 8-character prefix), passwords and tokens are never logged, and `/healthz` polling logs at `debug` only. For streaming downloads the status reflects the response headers — a transfer that dies mid-stream still shows `status=200` with a short `bytes_out`.

## Metrics

Optionally, GoblinFTP exposes Prometheus metrics on a **dedicated port** — separate from the app server, so Caddy never proxies it and it stays unreachable from outside the container unless you publish the port to your monitoring network.

| Variable | Default | Description |
|---|---|---|
| `GFTP_METRICS_ENABLED` | `false` | Enable the Prometheus `/metrics` endpoint |
| `GFTP_METRICS_PORT` | `9091` | Port for the metrics-only listener (separate from the app port) |

| Series | Type | Labels | Meaning |
|---|---|---|---|
| `gftp_http_requests_total` | counter | `method`, `path`, `status` | API requests by route template |
| `gftp_http_request_duration_seconds` | histogram | `method`, `path` | API request latency |
| `gftp_connect_attempts_total` | counter | `protocol`, `result` | Dial outcomes: `success`, `auth_failed`, `failed`, `throttled` |
| `gftp_transfer_bytes_total` | counter | `direction`, `protocol` | File bytes moved between browser and server (`upload`/`download`) |
| `gftp_frontend_errors_total` | counter | — | Accepted browser-error reports |
| `gftp_sessions_active` | gauge | — | Live sessions (computed at scrape time) |
| `gftp_connections_active` | gauge | `protocol` | Live FTP/SFTP connections (computed at scrape time) |
| `go_*`, `process_*` | — | — | Standard Go runtime / process collectors |

```yaml
# docker-compose: publish the metrics port to your monitoring network only
services:
  goblinftp:
    image: ghcr.io/darthsoup/goblinftp
    environment:
      GFTP_METRICS_ENABLED: "true"
    ports:
      - "9091:9091"   # ideally on an internal/monitoring network, not public

# prometheus.yml
scrape_configs:
  - job_name: goblinftp
    static_configs:
      - targets: ["goblinftp:9091"]
```

Note: the session/connection gauges are scrape-time snapshots of the in-memory session store. Sessions that expire by TTL disappear from the gauges immediately, even though the underlying FTP/SFTP connection may linger until the remote server times it out.

## S3 chunk staging

By default, chunked uploads are staged on local disk (`GFTP_DATA_DIR`) before being streamed to the connected FTP/SFTP server. Optionally, chunks can be staged in an S3-compatible bucket (MinIO, AWS S3, …) instead — useful for read-only containers, offloading disk I/O, or multi-replica deployments. This works identically for FTP and SFTP connections; nothing changes in the browser.

| Variable | Default | Description |
|---|---|---|
| `GFTP_S3_ENABLED` | `false` | Enable S3 chunk staging |
| `GFTP_S3_ENDPOINT` | — | Full endpoint URL incl. scheme, e.g. `http://minio:9000` or `https://s3.amazonaws.com` |
| `GFTP_S3_BUCKET` | — | Bucket for staged chunks (must already exist) |
| `GFTP_S3_ACCESS_KEY` / `GFTP_S3_SECRET_KEY` | — | Credentials (object read/write/delete + list is enough — no bucket-create needed) |
| `GFTP_S3_REGION` | `us-east-1` | Bucket region |
| `GFTP_S3_USE_PATH_STYLE` | `true` | Path-style addressing — keep `true` for MinIO, set `false` for AWS S3 |
| `GFTP_S3_PREFIX` | `gftp-uploads` | Key prefix for staged chunks |
| `GFTP_S3_TIMEOUT_SECS` | `60` | Per-request timeout for S3 calls |

Endpoint, bucket, and credentials are required when enabled — the server refuses to start without them. Credentials are env-only and never read from `settings.json`; use your orchestrator's secrets mechanism in production.

```bash
docker run -p 8080:80 \
  -e GFTP_S3_ENABLED=true \
  -e GFTP_S3_ENDPOINT=http://minio:9000 \
  -e GFTP_S3_BUCKET=gftp-chunks \
  -e GFTP_S3_ACCESS_KEY=minioadmin \
  -e GFTP_S3_SECRET_KEY=minioadmin \
  ghcr.io/darthsoup/goblinftp
```

Chunks live under `{prefix}/{uploadId}/` only for the duration of an upload and are deleted after the file is committed to the FTP/SFTP server. Uploads abandoned mid-flight (closed browser tab, cancelled transfer) are not reaped automatically — add a bucket lifecycle rule that expires objects under the prefix after a day:

```bash
# MinIO
mc ilm rule add --expire-days 1 --prefix "gftp-uploads/" local/gftp-chunks
```

```bash
# AWS S3 — save the JSON below as lifecycle.json, then:
aws s3api put-bucket-lifecycle-configuration --bucket gftp-chunks --lifecycle-configuration file://lifecycle.json
```

```json
{
  "Rules": [{
    "ID": "expire-abandoned-gftp-uploads",
    "Status": "Enabled",
    "Filter": { "Prefix": "gftp-uploads/" },
    "Expiration": { "Days": 1 }
  }]
}
```

## SSO login links

With `GFTP_SSO_ENABLED=true` and a `GFTP_SSO_SECRET` set, your application can generate one-time login links (`/?sso=<token>`) that connect users directly — no login form. Tokens are AES-256-GCM-encrypted, single-use, and short-lived.

```bash
just sso-link -host ftp.example.com -username alice -password s3cret -base-url https://files.example.com
```

See [`examples/sso/`](../examples/sso/) for the token format and ready-to-use generators in Go, Node.js, and PHP.

## settings.json

Key options (full schema in [`settings.example.json`](../settings.example.json)):

| Setting | Description |
|---|---|
| `connection.allowedTypes` | Restrict to `["ftp"]`, `["sftp"]`, or both |
| `connection.disableChmod` | Hide chmod UI |
| `connection.presetHost` / `presetPort` | Prefill the login form (panel deployments) |
| `connection.lockHost` | Make host + port read-only (requires `presetHost`) |
| `connection.passiveMode` | Default for the FTP passive-mode toggle |
| `editor.disabled` | Disable the file editor entirely |
| `editor.allowedExtensions` | Restrict editable file extensions |
| `access.allowedClientAddresses` | IP allowlist (empty = allow all) |
| `language` | Default UI language (users can override in settings) |
| `ui.showDotFiles` | Show hidden files by default (users can override) |

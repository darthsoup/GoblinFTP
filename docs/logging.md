# Logging

The `requestLogger` middleware emits one structured access line per request (method, path, status, duration, request ID, client IP, and the connected user/host once a session exists). Handlers report failures by stashing the `GFTPError` in the Echo context rather than returning it, so each failing line carries a machine-readable `error_code` plus the underlying `cause`. The `cause` is logged only; it is never serialized into the client-facing response envelope. Dynamic attributes pass through `logging.SafeLogAttrs` for redaction. Browser-side errors arrive at `POST /api/log/frontend` and are logged as a separate `frontend error` line.

| Variable | Default | Description |
|---|---|---|
| `GFTP_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. At `warn` and above the successful-request lines are suppressed while failures remain. |
| `GFTP_LOG_FORMAT` | `json` | `json` or `text`. Any other value fails startup. |
| `GFTP_LOG_FILE` | (none) | Path to an additional rotating file sink (lumberjack). Stdout is always written regardless. |
| `GFTP_LOG_FILE_MAX_SIZE_MB` | `10` | Rotate once the file reaches this size. Must be greater than 0 or startup fails. |
| `GFTP_LOG_FILE_MAX_BACKUPS` | `5` | Rotated files to retain. Must be 0 or greater. |
| `GFTP_LOG_FILE_MAX_AGE_DAYS` | `0` | Delete rotated files older than this many days. Must be 0 or greater; `0` disables age-based deletion. |
| `GFTP_LOG_FRONTEND` | `true` | Accept browser-error reports on `POST /api/log/frontend` (public, CSRF-exempt, per-IP rate-limited). Disabled only by the exact string `false`. |

```bash
# Docker-native: read the container output and ship it via your log driver, Loki, or ELK.
docker logs -f goblinftp

# Optional file sink on the data volume, for setups without a log collector.
docker run -p 8080:80 \
  -e GFTP_LOG_FILE=/app/data/logs/gftp.log \
  -v gftp-data:/app/data \
  ghcr.io/darthsoup/goblinftp
```

## Redaction and edge cases

- The full session ID is never logged; only an 8-character prefix appears.
- Passwords and tokens are never logged.
- `/healthz` polling logs at `debug`, so it is silent at the default `info` level.
- Streaming downloads log the committed response status. Once headers are sent as `200`, a transfer that dies mid-stream still logs `status=200` with a short `bytes_out`.

## See also

- [Configuration](configuration.md) for the full environment-variable reference.
- [Metrics](metrics.md) for the Prometheus endpoint.

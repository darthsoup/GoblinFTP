# Logging

The `requestLogger` middleware emits one structured access line per request (method, path, status, duration, request ID, client IP, and the connected user/host once a session exists). Handlers report failures by stashing the `GFTPError` in the Echo context rather than returning it, so each failing line carries a machine-readable `error_code` plus the underlying `cause`. The `cause` is logged only; it is never serialized into the client-facing response envelope. Dynamic attributes pass through `logging.SafeLogAttrs` for redaction. Browser-side errors arrive at `POST /api/log/frontend` and are logged as a separate `frontend error` line.

<!-- confgen:begin env-table "Logging" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_LOG_LEVEL` | `info` | Log level. |
| `GFTP_LOG_FORMAT` | `json` | Log output format. |
| `GFTP_LOG_FILE` | (none) | Optional rotating file sink in addition to stdout; empty disables it. |
| `GFTP_LOG_FILE_MAX_SIZE_MB` | `10` | Rotate the log file when it exceeds this size. |
| `GFTP_LOG_FILE_MAX_BACKUPS` | `5` | Rotated files to keep; 0 keeps all. |
| `GFTP_LOG_FILE_MAX_AGE_DAYS` | `0` | Days to keep rotated files; 0 keeps them indefinitely. |
| `GFTP_LOG_FRONTEND` | `true` | Forward browser errors to POST /api/log/frontend. |
<!-- confgen:end -->

At `warn` and above the successful-request lines are suppressed while failures remain. Stdout is always written; the `GFTP_LOG_FILE_*` knobs apply only when a file sink is set. `POST /api/log/frontend` is public, CSRF-exempt, and per-IP rate-limited.

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

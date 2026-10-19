---
description: Read container logs, save them to a file, and find out why a request failed.
---

# Logging

GoblinFTP writes logs to the container output. To watch them:

```bash
docker logs -f goblinftp
```

Each request has a log entry with its result, duration, and request ID. Failed requests also include an error code and the cause. Browser errors appear as separate entries.

## Settings

<!-- confgen:begin env-table "Logging" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_LOG_LEVEL` | `info` | Messages to include: debug, info, warn, or error. |
| `GFTP_LOG_FORMAT` | `json` | Output format: json or text. |
| `GFTP_LOG_FILE` | (none) | Also write logs to this file. Empty keeps logs in the container output only. |
| `GFTP_LOG_FILE_MAX_SIZE_MB` | `10` | Start a new log file when the current file reaches this size in MB. |
| `GFTP_LOG_FILE_MAX_BACKUPS` | `5` | Old log files to keep. Set to 0 to keep all. |
| `GFTP_LOG_FILE_MAX_AGE_DAYS` | `0` | Days to keep old log files. Set to 0 for no time limit. |
| `GFTP_LOG_FRONTEND` | `true` | Send browser errors to the backend for logging. |
<!-- confgen:end -->

JSON is the default format. Use `text` if you prefer plain text. Set the level to `debug` for more detail, or `warn` to show failures without successful requests.

## Save logs to a file

Set `GFTP_LOG_FILE` to a writable path. GoblinFTP keeps writing to the container output too.

```bash
docker run -p 8080:8080 \
  -e GFTP_LOG_FILE=/app/data/logs/gftp.log \
  -v gftp-data:/app/data \
  ghcr.io/darthsoup/goblinftp
```

The size, backup count, and age settings control when old log files are replaced or removed.

## What logs include

- Passwords and tokens are not logged. Session IDs are shortened to eight characters.
- Logs can include the connected username, server address, and client IP.
- Health checks only appear at the `debug` level.
- A download that fails after it starts can still show `status=200`. Check the error and transferred byte count too.
- `GFTP_LOG_FRONTEND=false` stops browser-error entries and their metric counter. A separately configured browser Sentry SDK can still send reports.

Use `request_id` to match an error with its request or a [Sentry report](sentry.md).

## See also

- [Metrics](metrics.md) to monitor activity over time.
- [Error tracking](sentry.md) for crash and error reports.

# Metrics

GoblinFTP can expose Prometheus metrics on a dedicated listener, separate from the app server: its own `http.ServeMux` on a different port, with a 10-second `ReadHeaderTimeout`. It is never registered on the main Echo instance, so Caddy does not proxy it and it stays unreachable from outside the container unless you publish the port to your monitoring network.

| Variable | Default | Description |
|---|---|---|
| `GFTP_METRICS_ENABLED` | `false` | Enable the Prometheus `/metrics` endpoint. Enabled only by the exact string `true`. |
| `GFTP_METRICS_PORT` | `9091` | Port for the metrics-only listener. Must parse as an integer in `1-65535` or startup fails. |

| Series | Type | Labels | Meaning |
|---|---|---|---|
| `gftp_http_requests_total` | counter | `method`, `path`, `status` | API requests. `path` is the Echo route template, not the raw URL. |
| `gftp_http_request_duration_seconds` | histogram | `method`, `path` | API request latency. |
| `gftp_connect_attempts_total` | counter | `protocol`, `result` | Dial outcomes: `success`, `auth_failed`, `failed`, `throttled`. |
| `gftp_transfer_bytes_total` | counter | `direction`, `protocol` | Bytes to and from the FTP/SFTP server (`upload`/`download`), measured by `CountingReader`. |
| `gftp_frontend_errors_total` | counter | (none) | Accepted browser-error reports. |
| `gftp_sessions_active` | gauge | (none) | Live sessions, computed at scrape time. |
| `gftp_connections_active` | gauge | `protocol` | Live FTP/SFTP connections, computed at scrape time. |
| `go_*`, `process_*` | n/a | n/a | Standard Go runtime and process collectors. |

Label and counting details:

- The `path` label uses `c.Path()` (the matched route template), so cardinality stays bounded. Requests that match no route report `path="unmatched"`, and `/healthz` is excluded from HTTP metrics entirely. The metrics middleware sits between `RequestID` and `requestLogger` and never calls `c.Error`.
- `gftp_transfer_bytes_total` counts only bytes exchanged with the remote FTP/SFTP server. Chunk-staging reads and writes (local disk or S3) are not counted.
- The `active` gauges are scrape-time snapshots of the in-memory session store, produced by a custom collector (`SetConnectionSnapshot`), so there is no inc/dec drift. A session that expires by TTL disappears immediately, even though the underlying FTP/SFTP connection may linger until the remote server times it out.

```yaml
# docker-compose: publish the metrics port to your monitoring network only.
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

## See also

- [Configuration](configuration.md) for the full environment-variable reference.
- [Logging](logging.md) for the structured request log.

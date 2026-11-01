---
description: Monitor requests, connections, and file transfers with Prometheus.
---

# Metrics

GoblinFTP can share activity counters with Prometheus. Use them to watch request rates, connection failures, and transfer traffic over time.

Metrics are off by default. They use their own port, separate from the web app.

## Enabling it

<!-- confgen:begin env-table "Metrics" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_METRICS_ENABLED` | `false` | Allow Prometheus to collect metrics from /metrics. |
| `GFTP_METRICS_PORT` | `9091` | Port for /metrics, separate from the app port. |
<!-- confgen:end -->

Add `GFTP_METRICS_ENABLED=true` to your `.env` file and restart GoblinFTP. If Prometheus is on the same Docker network, it can collect metrics from `gftp:9091`:

```yaml
scrape_configs:
  - job_name: goblinftp
    static_configs:
      - targets: ["gftp:9091"]
```

Here, `gftp` is the Compose service name. If Prometheus runs outside that network, publish port 9091 on a private monitoring interface and use the host address instead.

For a local development server, enable metrics with `GFTP_METRICS_ENABLED=true just dev-be`. In another terminal, check the output:

```bash
curl -s localhost:9091/metrics | grep gftp_
```

Use port 9091, not the app's port 8080. On the app port, `/metrics` can return the web interface instead of metrics. Keep `GFTP_METRICS_PORT` different from the backend's `GFTP_PORT`.

### The endpoint is unauthenticated

Anyone who can reach the metrics port can read it. Keep it on a private network or protect it with your own proxy. GoblinFTP's client-IP restrictions do not cover this port.

No usernames, server addresses, remote file paths, or session IDs are included as metric labels.

## Available metrics

A **counter** adds up events since startup. A **gauge** shows the current count. A **histogram** groups request times into ranges.

| Metric | Type | What it shows |
|---|---|---|
| `gftp_http_requests_total` | Counter | API requests by method, route, and status |
| `gftp_http_request_duration_seconds` | Histogram | Request times by method and route |
| `gftp_connect_attempts_total` | Counter | Connection results by protocol |
| `gftp_transfer_bytes_total` | Counter | Bytes transferred on the paths listed below |
| `gftp_frontend_errors_total` | Counter | Accepted browser-error reports |
| `gftp_sessions_active` | Gauge | Active login sessions |
| `gftp_connections_active` | Gauge | Active FTP and SFTP connections; FTPS is currently missing |
| `go_*`, `process_*` | Various | Go runtime and process statistics; process statistics are Linux-only |

Routes use templates such as `/themes/:tenant/:file`, not individual filenames.

### Connection results

| Result | Meaning |
|---|---|
| `success` | Connected and created a session |
| `auth_failed` | Server rejected the login details |
| `host_key_mismatch` | SFTP server key changed |
| `host_key_prompt` | User was asked to trust a new SFTP server key |
| `tls_failed` | Could not establish a secure FTPS connection |
| `failed` | Another connection error, such as a timeout or unknown hostname |
| `throttled` | Login temporarily blocked after repeated failures |

Use this counter to spot trends, rather than to count exact logins. Requests rejected before connecting are not counted. A connection that fails during session setup is not counted either. Trusting a new SFTP key produces two results: a prompt, then the connection result.

### Limits for login links

The connection made through an SSO link is counted. Rejected links, such as expired or reused links, have no separate result counter. Check logs for those failures.

SSO connections do not use the manual-login throttle and do not produce a `throttled` result.

### Label values

The `direction` label is `upload` or `download`. The `protocol` label is usually `ftp`, `ftps`, or `sftp`. Transfer counts can also use `unknown` if the session has no recorded protocol.

### Which transfers are counted

| Operation | Counted? |
|---|---|
| Single-file upload | Yes |
| Final step of a chunked upload | Yes |
| Single-file download | Yes |
| ZIP download | Yes, before compression |
| Extracting archives | No |
| Creating archives on the server | No |
| Copying files on the server | No |
| Opening or saving files in the editor | No |
| Temporary upload storage, on disk or S3 | No |

Failed transfers still count the bytes that moved. ZIP downloads count the original file bytes read from the server, which can differ from the ZIP size sent to the browser.

This is a partial traffic count. Do not use it as a complete bandwidth or billing measurement.

### Request counts and timings

Health checks at `/healthz` are not counted. Prometheus scrapes use a separate server, so they are not counted in `gftp_http_*` either.

The request-time histogram has a highest fixed boundary of 10 seconds. Longer requests share its final bucket, so percentiles are not reliable for long transfers. Exclude transfer routes from latency alerts, or use transfer throughput instead.

### Active sessions and connections

Session and connection counts are read directly from the session store whenever Prometheus collects metrics. A session can exist without a remote connection, so the session count can be higher.

Expired sessions disappear from these counts, even if the remote server has not closed its connection yet.

## Current limits

Keep these limits in mind when setting alerts:

- **FTPS connections are missing from the active-connection gauge.** Other protocol-based counters include FTPS.
- **Unusual client input can create extra time series.** Protocol capitalization and custom HTTP methods can produce additional label values. Watch for unexpected growth on public instances.
- **A scrape can be interrupted during shutdown.**
- **Some descriptions in the metrics output are outdated.** Use the coverage notes on this page when interpreting them.

If metrics are unavailable, check the startup logs. A port error can leave the app running without metrics. If the metrics and backend ports are the same, the app can fail to start.

## Example queries

::: details Prometheus query examples

```promql
# Request rate by route
sum by (path) (rate(gftp_http_requests_total[5m]))

# Server-error ratio
sum(rate(gftp_http_requests_total{status=~"5.."}[5m]))
  / sum(rate(gftp_http_requests_total[5m]))

# p95 API latency, excluding transfers that can exceed the histogram range.
histogram_quantile(0.95,
  sum by (le, path) (rate(gftp_http_request_duration_seconds_bucket{
    path!~"/api/files/(upload.*|download|download-zip|compress|extract|read|write)"}[5m])))

# Bytes per second for the transfers that are counted
sum by (direction) (rate(gftp_transfer_bytes_total[5m]))

# Failed logins by protocol
sum by (protocol) (rate(gftp_connect_attempts_total{result="auth_failed"}[5m]))

# Logins blocked after repeated failures
increase(gftp_connect_attempts_total{result="throttled"}[15m]) > 0

# Changed SFTP server keys
increase(gftp_connect_attempts_total{result="host_key_mismatch"}[1h]) > 0

# Unexpected connection-type labels
count(count by (protocol) (gftp_connect_attempts_total)) > 3
```

:::

Watch `up{job="goblinftp"} == 0` for a failed metrics endpoint and `scrape_duration_seconds` for slow collection.

Counters reset and active sessions drop to zero when GoblinFTP restarts. Allow a grace period in your alert rules. Rising browser-error counts without more server errors can point to a browser-side bug.

## Adding a metric

::: details Notes for developers

1. Declare and register the metric in `backend/internal/metrics/metrics.go`. Use the existing namespace constant.
2. Update counters through `h.metrics`. The handler has a registry even when the metrics endpoint is disabled.
3. Derive current-state gauges from a collector, following `connCollector` in `collector.go`.
4. Use fixed label values. Never copy unrestricted user input into a label, and include every supported protocol when listing values.
5. Pass transfer counters through to the actual reader or writer. A nil counter records nothing.

Tests in `backend/internal/metrics/metrics_test.go` cover collectors and byte counting. Tests in `backend/internal/api/metrics_test.go` cover HTTP behavior and metric naming.

Disabling the endpoint does not stop internal counters from being updated.

:::

## See also

- [Logging](logging.md) to investigate individual requests.
- [Error tracking](sentry.md) for application errors.
- [S3 upload storage](s3-staging.md) for temporary uploads.

The metrics listener defaults to loopback. For a separate Prometheus container, explicitly set `GFTP_METRICS_BIND_ADDRESS=0.0.0.0` and restrict access to the monitoring network.

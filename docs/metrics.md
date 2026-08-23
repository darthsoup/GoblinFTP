# Metrics

GoblinFTP can expose Prometheus metrics on a dedicated listener, separate from the app server: its own `http.ServeMux` on a different port, with a 10-second `ReadHeaderTimeout`. It is never registered on the main Echo instance, so Caddy does not proxy it and it stays unreachable from outside the container unless you publish the port to your monitoring network.

The feature is opt-in and off by default. Everything lives in `backend/internal/metrics`, which owns a private `prometheus.Registry` rather than the global default registry, so tests can assert against a known set of series without cross-contamination.

Read the [Known gaps](#known-gaps) section before you build alerts on any of this. Several series measure less than their names suggest.

## Enabling it

<!-- confgen:begin env-table "Metrics" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_METRICS_ENABLED` | `false` | Enable the Prometheus /metrics listener. |
| `GFTP_METRICS_PORT` | `9091` | Port for the metrics-only listener. |
<!-- confgen:end -->

To enable metrics with the repo's compose setup: set `GFTP_METRICS_ENABLED=true` in `.env` (it reaches the container via `env_file`) and uncomment the `9091:9091` port mapping in `docker-compose.yml`, ideally publishing it on an internal monitoring network rather than publicly. Then point Prometheus at it:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: goblinftp
    static_configs:
      # "gftp" is the compose SERVICE name, which is the DNS name on the compose
      # network. From outside the network, use the published host port instead.
      - targets: ["gftp:9091"]
```

On startup the backend logs `metrics listening` with the port. That line is emitted *before* the socket is bound, so it is not proof of success: on a bind failure it is followed immediately by `metrics server stopped` with the error, and the application keeps running without metrics. A scrape target that never comes up is a log check, not a crash.

One exception to that tolerance: nothing validates `GFTP_METRICS_PORT` against `GFTP_PORT`. They are each range-checked in isolation, and the metrics goroutine binds first, so setting both to the same value makes the *main* server fail to start and the process exit.

Note that `/metrics` on the normal app port (8080) does not fail loudly. Caddy has no route for it, so the request falls through to the SPA catch-all and returns HTTP 200 with `index.html`. A `curl` that comes back with HTML rather than an error means you are on the wrong port, not that the endpoint is broken.

To check it locally without Docker:

```bash
cd backend && GFTP_METRICS_ENABLED=true GFTP_DATA_DIR=data go run ./cmd/gftp
curl -s localhost:9091/metrics | grep gftp_
```

On macOS that output has `go_*` but no `process_*`: the process collector is Linux-only. The registry is not broken.

### The endpoint is unauthenticated

`/metrics` has no authentication, no token, and no IP allowlist. Anything that can reach the port can read every series. `GFTP_ACCESS_ALLOWED_IPS` and `GFTP_ACCESS_TRUSTED_PROXIES` apply to the Echo server only and do not cover this listener. The server binds `:<port>`, meaning every interface, and there is no key to bind it to a single address.

The protection model is therefore purely network-level: do not publish the port to the internet, keep it on a monitoring network, or put your own authenticating proxy in front of it. The "unreachable from outside the container" property above depends on bridge networking. With `network_mode: host`, host-network Kubernetes, or a bare binary, enabling metrics exposes the port directly.

The exposure is operational rather than sensitive. No hostnames, usernames, remote paths, or session identifiers become label values. The `path` label is the Echo route template, never the raw URL, and a test asserts that a user-supplied URL can never appear as a label value. Two labels are fed from client input, `protocol` and `method`, both described under [Known gaps](#known-gaps).

## Series

| Series | Type | Labels | Meaning |
|---|---|---|---|
| `gftp_http_requests_total` | counter | `method`, `path`, `status` | API requests. `path` is the Echo route template, not the raw URL. |
| `gftp_http_request_duration_seconds` | histogram | `method`, `path` | API request latency, default Prometheus buckets (5ms to 10s). |
| `gftp_connect_attempts_total` | counter | `protocol`, `result` | Dial outcomes. Not every request produces one, see below. |
| `gftp_transfer_bytes_total` | counter | `direction`, `protocol` | Bytes on *some* remote transfer paths. Coverage is partial, see below. |
| `gftp_frontend_errors_total` | counter | (none) | Accepted browser-error reports on `/api/log/frontend`. |
| `gftp_sessions_active` | gauge | (none) | Live sessions, computed at scrape time. |
| `gftp_connections_active` | gauge | `protocol` | Live connections, computed at scrape time. FTP and SFTP only, never FTPS. |
| `go_*`, `process_*` | n/a | n/a | Standard Go runtime and process collectors. |

### Connect results

`result` on `gftp_connect_attempts_total` has seven values:

| `result` | Fires when |
|---|---|
| `success` | The dial succeeded and a session was created. |
| `auth_failed` | The server rejected the credentials. |
| `host_key_mismatch` | SFTP host key did not match `known_hosts`. |
| `host_key_prompt` | SFTP host key is unknown and the user was asked to confirm it. Neither a failure nor a success. |
| `tls_failed` | FTPS TLS negotiation failed. |
| `failed` | Any other dial failure (refused, timeout, DNS, unrecognized protocol string). |
| `throttled` | The login throttle rejected the attempt before dialing. |

**Do not read the sum of this counter as "login attempts".** It is neither that nor "logins":

- Requests rejected before the dial fire nothing: malformed JSON, a disallowed connection type, a missing host or username, a port outside 1 to 65535, a host-lock violation, an IP-allowlist rejection.
- A dial that succeeds but then fails while setting up the session (working-directory probe, CSRF token, session creation) also fires nothing, even though a real login happened on the remote server.
- One SFTP trust-on-first-use login fires *two* increments: `host_key_prompt` on the first leg, then `success` or a failure on the confirmation retry.

Use it for the *shape* of failures over time, not for absolute counts.

### SSO logins are only half instrumented

SSO has two stages, and only the second one counts. `POST /api/auth/sso-connect` increments the six non-throttled results exactly like the password path. The token stage (`GET /?sso=<token>`) increments nothing at all: a disabled SSO config, an expired token, an invalid token, a replayed one-time token, a disallowed connection type, and a host-lock rejection all redirect to `/login?sso_error=...` without touching any counter.

Rejected SSO tokens are therefore invisible in every series except `gftp_http_requests_total`, so token abuse cannot be alerted on directly. Related: the SSO connect path performs no throttle check and never records a failure with the throttle, so it produces no `throttled` value and SSO-minted credentials are not rate limited the way password logins are.

### Label values

`direction` is `upload` or `download`.

`protocol` is normally `ftp`, `ftps`, or `sftp`. On `gftp_transfer_bytes_total` it is read from the session and can also be `unknown` when the session has no protocol recorded. On `gftp_connect_attempts_total` it is the client-supplied string, which has a cardinality caveat under [Known gaps](#known-gaps).

### What `gftp_transfer_bytes_total` actually counts

Only four transfer paths are instrumented, out of eleven that move bytes to or from the remote server:

| Counted | Path |
|---|---|
| yes | Single-file upload (`POST /api/files/upload`) |
| yes | Chunked upload commit |
| yes | Single-file download |
| yes | ZIP download (`/api/files/download-zip`), counting bytes read from the server while building the archive |

The ZIP row counts source bytes pulled from the server, not the compressed bytes delivered to the browser, so the two normally differ. Usually the counter is higher, but for already-compressed content (JPEG, MP4, ZIP) the archive can end up slightly larger than the counted source. A build that fails partway still counts what it read even though the user receives nothing.

Uncounted, despite moving real FTP/SFTP bytes: archive **extraction** (both ZIP and TAR, up to the 512 MB decompressed budget per request, the largest uncounted write path), server-side **compress** (`POST /api/files/compress`, which deliberately passes a nil counter, so neither the source reads nor the resulting upload count), server-side **copy** (recursed over entire trees), and both **editor** operations (opening a file and saving it).

Two further properties of what is counted:

- Chunk-staging reads and writes are excluded, whether staging is local disk or S3. A chunked upload counts each byte once, when it is finally sent to the remote server, not twice.
- Bytes are counted as they are read, so a transfer that dies mid-stream still counts what actually moved. This is deliberate: it measures traffic, not completed transfers.

Treat this counter as a lower bound on remote traffic, not a measurement of it.

### HTTP series exclusions

`/healthz` is skipped entirely. The skip matches the raw URL path regardless of method, so even a `PUT /healthz` returning 405 is invisible to metrics while still appearing in the access log. (It is the image `HEALTHCHECK` that polls it, not the entrypoint.)

`path="unmatched"` is rarer than it looks. Echo registers catch-all routes for the `/api` group, so anything under `/api` that matches no concrete route is labeled `path="/api/*"` rather than `unmatched`, and it is not always a 404: `POST /api/files` hits the catch-all with CSRF middleware attached and reports 401. A request that matches a parameterized route keeps that route's template even when the handler 404s, so a missing theme file reports `path="/themes/:tenant/:file"`. Only requests outside every registered prefix reach `unmatched`.

One caveat on `gftp_http_request_duration_seconds`: it uses the default Prometheus buckets, whose highest finite boundary is 10 seconds. The middleware times the whole handler including the streamed body, so a transfer slower than 10 seconds is counted only in the implicit `+Inf` bucket and becomes indistinguishable from any other slow request. Short transfers still land in the normal buckets. Where observations pile up in `+Inf`, `histogram_quantile` returns `+Inf` rather than a number, so quantiles on transfer routes are not usable and only `_sum` and `_count` carry information.

There is no wider-bucket histogram today. If you need transfer latency, either add a second histogram with custom buckets for those routes or infer health from `rate(gftp_transfer_bytes_total[5m])` instead.

### The scrape-time gauges

`gftp_sessions_active` and `gftp_connections_active` are not incremented and decremented at call sites. A custom collector walks the in-memory session store once per scrape and emits both series from that single snapshot, so the numbers cannot drift out of sync the way paired inc/dec calls can. Before the snapshot function is wired the gauges read zero rather than disappearing.

A session counts as a connection when it holds a live `transfer.Client`. Sessions and connections differ when a session exists without a connection, so `gftp_sessions_active` is normally greater than or equal to the sum of `gftp_connections_active`.

Because the source is the session store, a session that expires by TTL disappears from the gauges immediately, even though the underlying FTP/SFTP connection may linger until the remote server times it out.

## Known gaps

These are current behaviors, verified against the code. They are documented rather than fixed, so alerts are not built on false assumptions.

**FTPS is missing from `gftp_connections_active`.** The snapshot counts FTPS connections, but the collector emits only the `ftp` and `sftp` series, so the FTPS count is discarded at emission and is not folded into `ftp`. Three live sessions (one per protocol) report `gftp_sessions_active 3` with `ftp=1` and `sftp=1` and no FTPS series at all. The other two protocol-labeled series do report `ftps` correctly.

**The `protocol` label on `gftp_connect_attempts_total` is unvalidated client input.** The allowed-types check is case-insensitive, but the dialer switches on the exact lowercase string. So a request with `"protocol": "FTP"` passes the allowlist, mints a brand new label series, and always lands on `result="failed"`. The label domain is bounded by case permutations of the three names rather than by configuration, and it is attacker-influenced. If you scrape a publicly reachable instance, watch for series growth here.

**The `method` label is unbounded.** Go accepts any RFC-compliant token as an HTTP method and the middleware records it verbatim, so an unauthenticated `curl -X ZZQQ1 http://host/api/files` creates a fresh `gftp_http_requests_total{method="ZZQQ1"}` series plus a full set of histogram buckets. Series created this way never expire while the process lives.

**Metrics shutdown shares an already-consumed context.** On SIGTERM the main server drains first with a 20-second budget, then the metrics listener is shut down with that same context rather than a fresh one. If the main drain used the full grace, the metrics shutdown gets an expired context and returns immediately, cutting in-flight scrapes. The error is discarded, so this is silent.

**Some HELP strings on the wire are stale.** The `gftp_connect_attempts_total` help text lists only four of the seven `result` values, and `gftp_transfer_bytes_total` describes itself as bytes "between browser and the connected server" when it measures the remote side only, and only on some paths. Where the served HELP text and this page disagree, this page is current.

## Useful queries

```promql
# Request rate by route
sum by (path) (rate(gftp_http_requests_total[5m]))

# Server-error ratio
sum(rate(gftp_http_requests_total{status=~"5.."}[5m]))
  / sum(rate(gftp_http_requests_total[5m]))

# p95 latency for the interactive API. The transfer routes are excluded on
# purpose: their observations sit in +Inf and would return +Inf here.
histogram_quantile(0.95,
  sum by (le, path) (rate(gftp_http_request_duration_seconds_bucket{
    path!~"/api/files/(upload.*|download|download-zip|compress|extract|read|write)"}[5m])))

# Throughput in bytes per second, by direction (instrumented paths only)
sum by (direction) (rate(gftp_transfer_bytes_total[5m]))

# Failed logins by protocol
sum by (protocol) (rate(gftp_connect_attempts_total{result="auth_failed"}[5m]))

# Someone is hitting the login throttle
increase(gftp_connect_attempts_total{result="throttled"}[15m]) > 0

# Host key mismatches, which may mean a man-in-the-middle or a rebuilt server
increase(gftp_connect_attempts_total{result="host_key_mismatch"}[1h]) > 0

# Unexpected protocol label values, which means someone is fuzzing the connect body
count(count by (protocol) (gftp_connect_attempts_total)) > 3
```

`gftp_frontend_errors_total` rising without a matching rise in backend 5xx usually means a client-side regression, since only accepted reports are counted and rejected payloads are not.

Two things to keep in mind when turning these into alerting rules. The metrics listener is not the Echo server, so scrapes never appear in `gftp_http_*` and there is no `promhttp_*` series either: watch `up{job="goblinftp"} == 0` and `scrape_duration_seconds` for endpoint health rather than looking for the scrape in the app's own counters. And every counter resets to zero on restart while the session store empties, so a bare `increase(...) > 0` re-fires after each deploy and the gauges legitimately drop to 0. Give rules a `for:` window and expect that dip.

## Adding a metric

1. Declare the series in `Metrics` and construct it in `New()` in `backend/internal/metrics/metrics.go`, then add it to the `MustRegister` call. Use the `namespace` constant rather than writing the `gftp_` prefix by hand.
2. Increment it at the call site through `h.metrics`. Handlers always hold a non-nil instance, even when metrics are disabled, so no nil guard is needed. (`metrics.CountingReader` is the exception that deliberately tolerates a nil counter.)
3. For anything derived from live state, prefer a scrape-time collector over inc/dec so it cannot drift. `connCollector` in `collector.go` is the pattern. If a collector enumerates label values, make sure that list matches every value the producer can emit; the FTPS gap above is exactly that mistake.
4. Keep label values bounded and never derived from user input. Route templates and fixed result strings are safe. Note that the shipped `protocol` label is *not* a good example to copy: it passes a client-supplied string through verbatim.
5. When wrapping a transfer, pass the counter all the way down. Several paths take a nil counter and silently count nothing.

Tests live in `backend/internal/metrics/metrics_test.go` (collector and `CountingReader`) and `backend/internal/api/metrics_test.go` (end-to-end through the router, using `api.WithMetrics(m)` to inject a known registry). `TestRegistryNamingLint` runs the Prometheus naming linter over the whole registry, so a badly named or badly suffixed series fails the build.

Note that instrumentation runs whether or not `GFTP_METRICS_ENABLED` is set. When metrics are disabled the handler still creates its own registry and still increments counters; the only difference is that nothing serves them. Disabling metrics is not a performance optimization.

## See also

- [Configuration](configuration.md) for the full environment-variable reference.
- [Logging](logging.md) for the structured request log.
- [Error tracking](sentry.md) for exception reporting and request tracing.
- [S3 chunk staging](s3-staging.md) for why staging bytes are excluded from transfer counters.

# Error tracking (Sentry)

GoblinFTP reports to [Sentry](https://sentry.io) from two places: the Go backend, and optionally the browser SPA. Both are off until you set a DSN, and neither is required to run the app.

Sentry here is scoped to defects: crashes, internal errors, and failing dependencies. It is deliberately not the place to watch end-customer connection problems, because those are business events rather than bugs. See [Which audience looks where](#which-audience-looks-where).

## Backend

<!-- confgen:begin env-table "Error tracking (Sentry)" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_SENTRY_DSN` | (none) | Backend DSN. Empty disables backend reporting. |
| `GFTP_SENTRY_ENVIRONMENT` | (none) | Environment tag passed through verbatim. |
| `GFTP_SENTRY_RELEASE` | (none) | Release tag; defaults to the build version. |
| `GFTP_SENTRY_SAMPLE_RATE` | `0` | Performance-trace sample rate between 0 and 1. 0 disables tracing; errors are unaffected. |
| `GFTP_SENTRY_ERROR_SAMPLE_RATE` | `1` | Fraction of error events sent, between 0 and 1. Lower it to cap quota burn when a remote server flaps. |
| `GFTP_SENTRY_CAPTURE_REMOTE_ERRORS` | `false` | Also report faults on the far side of the connection (unreachable host, dropped connection, timeout, TLS, remote disk full). Off by default: those are end-user and remote-server conditions, already covered by the access log and Prometheus. |
| `GFTP_SENTRY_SEND_SESSION_CONTEXT` | `false` | Attach the FTP username, remote host, and session prefix to events. Off by default: it sends end-customer identifiers to a third party. |
<!-- confgen:end -->

`GFTP_SENTRY_RELEASE` defaults to the build version, so tagged images report their tag and branch builds report `dev`.

### What gets reported

| Situation | Reported | Level |
|---|---|---|
| Panic in a handler | yes, with stack trace | fatal |
| Panic in a background goroutine (sweeper, metrics listener, server) | yes, then the process still dies | fatal |
| `ERR_INTERNAL`, `ERR_OPERATION_FAILED`, `ERR_LIST_FAILED`, any other 5xx | yes | error |
| `ERR_HOST_KEY_MISMATCH` | always, tagged `security=host_key_mismatch` | warning |
| Unreachable host, lost connection, timeout, TLS failure, remote disk full | only with `GFTP_SENTRY_CAPTURE_REMOTE_ERRORS=true` | warning |
| Failed logins, throttling, permission denied, missing files (any 4xx) | no | n/a |
| Browser errors relayed through `POST /api/log/frontend` | yes, tagged `source=frontend` | error |

A host key change is reported whatever the remote-error setting, because it can mean a man in the middle rather than a rekeyed server.

### What is on an event

Every request event carries the fields that make it actionable:

- `request_id`: the same value as the `X-Request-Id` response header and the `request_id` field of the access-log line. This is how you get from a Sentry issue to its log line.
- `route`: the Echo route template, never the concrete path. Grouping uses an explicit fingerprint of method plus route plus error code, so one defect stays one issue instead of fanning out per tenant, per file, or per upload ID.
- `error_code`: the `GFTPError` code, matching `error_code` in the log line.
- `protocol`: `ftp`, `ftps`, or `sftp` when a session exists.
- The exception value holds the underlying cause, the raw server or network text that `classify()` keeps out of the API response.

With `GFTP_SENTRY_SEND_SESSION_CONTEXT=true` the event also carries the FTP username, the remote host, and the 8-character session prefix.

### Privacy

By default no end-customer identifier is sent:

- `SendDefaultPII` is off, and `event.User` is cleared in `BeforeSend` regardless of what any caller set.
- sentry-go scrubs the attached request by parameter and header **name**. That is what keeps `?sso=` (which decrypts to an FTP password), `?token=`, the `gftp_session` cookie, and `X-CSRF-Token` out of the payload. `TestRequestSecretsAreFiltered` pins this, so renaming one of those parameters to a name outside sentry-go's denylist fails the build.
- Usernames, remote hosts, and session prefixes are sent only behind `GFTP_SENTRY_SEND_SESSION_CONTEXT`.
- The client IP is never attached.

One thing to know before you enable Sentry at all: the underlying **cause** of an error is always attached, and a dial or TLS error commonly names the remote host (for example `dial tcp ftp.customer.example:21: connect: connection refused`). It never contains a password. If that is unacceptable for your deployment, leave the DSN unset and use the access log instead.

### Tracing

`GFTP_SENTRY_SAMPLE_RATE` controls performance tracing, which is off by default. When set above 0 each request becomes a transaction named by its route template, and an incoming `sentry-trace` header is continued, so a browser trace and its backend trace join up. Request latency is also available in [Prometheus](metrics.md) without any sampling, so enable tracing only when you want the distributed view.

`GFTP_SENTRY_ERROR_SAMPLE_RATE` is separate and applies to errors. Setting it to 0 sends nothing.

## Frontend

The SPA has its own DSN, read at **build time** and baked into the static output. All `GFTP_*` variables are read at process start instead, so these are set differently.

| Variable | Default | Description |
|---|---|---|
| `NUXT_PUBLIC_SENTRY_DSN` | (none) | Browser DSN. Empty disables the browser SDK. |
| `NUXT_PUBLIC_SENTRY_ENVIRONMENT` | (none) | Environment tag for browser events. |
| `NUXT_PUBLIC_SENTRY_RELEASE` | `$VERSION` | Release tag for browser events. Match it to the backend release so both sides group together. |
| `NUXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE` | `0` | Browser trace sample rate between 0 and 1. |

Browser URLs are scrubbed before anything leaves the page (`app/utils/sentryScrub.ts`): the values of query parameters whose name contains `sso`, `token`, `session`, `csrf`, `auth`, `password`, `secret`, `key`, or `path` are replaced with `[Filtered]`, in fetch and navigation breadcrumbs as well as `event.request.url`. Without this the 15-minute download token from `GET /api/files/download?token=…` and the customer's remote file paths would be recorded verbatim in the breadcrumb trail of every later event.

### The two browser paths

Browser errors can reach you two ways, and they do not duplicate:

1. `POST /api/log/frontend`, controlled by `GFTP_LOG_FRONTEND` (on by default). The backend logs the report and counts it in `gftp_frontend_errors_total`.
2. The browser Sentry SDK, when `NUXT_PUBLIC_SENTRY_DSN` is set.

`app/plugins/error-reporter.client.ts` is the single funnel: the browser SDK's own `GlobalHandlers` integration is disabled, so one throw is captured exactly once. When a browser DSN is configured the SPA files the event and sends its event ID with the report; the backend logs that ID as `sentry_event_id` and skips its own relay. With no browser DSN the backend relays the report instead, so browser errors still reach Sentry with only `GFTP_SENTRY_DSN` set.

The reporter's dedupe and its cap of 20 reports per page load therefore bound Sentry volume as well as log volume. Sentry capture does not depend on `GFTP_LOG_FRONTEND`: turning that off stops the backend log and the `gftp_frontend_errors_total` counter, not the browser SDK.

### Source maps

`nuxt.config.ts` emits hidden client source maps: they are generated for upload but not referenced from the bundles, so browsers never fetch them and your source is not published. Without uploading them, browser stack traces stay minified and are hard to read.

To upload them, add the Sentry Nuxt module to `modules` in `nuxt.config.ts` and provide your org's credentials at build time:

```bash
SENTRY_AUTH_TOKEN=… SENTRY_ORG=… SENTRY_PROJECT=… pnpm --filter goblinftp-frontend run generate
```

This is left unconfigured because it needs organization-specific secrets.

`docker/Dockerfile` deletes every `*.map` from the generated output after the build, so the shipped image never carries them. Add your upload step before that `find … -delete` line. The same Dockerfile accepts `NUXT_PUBLIC_SENTRY_DSN`, `NUXT_PUBLIC_SENTRY_ENVIRONMENT`, and `NUXT_PUBLIC_SENTRY_RELEASE` as build args:

```bash
docker build -f docker/Dockerfile \
  --build-arg VERSION=v1.2.3 \
  --build-arg NUXT_PUBLIC_SENTRY_DSN=https://… \
  --build-arg NUXT_PUBLIC_SENTRY_ENVIRONMENT=production \
  -t goblinftp .
```

Because the browser DSN is baked in, a different DSN or environment means a different image. If you need one image across environments, leave `NUXT_PUBLIC_SENTRY_DSN` unset and let the backend relay browser errors instead.

## Which audience looks where

Sentry answers one question: is the application itself broken. The other two questions have better homes, which is why the defaults keep them out of Sentry.

| Question | Audience | Where to look |
|---|---|---|
| Did the app crash or throw? | Developers | Sentry, filtered to level `error` and `fatal` |
| Is a dependency failing (S3 staging, disk, memory)? | Administrators | Sentry, plus [metrics](metrics.md) |
| Is a specific customer failing to connect? | Customer service | The access log: `error_code`, `host`, `user`, `remote_ip` on the request line ([logging](logging.md)) |
| Are connection failures rising overall? | Administrators | `gftp_connect_attempts_total` by `protocol` and `result` ([metrics](metrics.md)) |
| Which release introduced this? | Developers | Sentry release tag, set from the build version |

Routing per-customer connection failures into Sentry buries genuine defects under end-user mistakes. If you need them there for a specific investigation, turn on `GFTP_SENTRY_CAPTURE_REMOTE_ERRORS` for the duration; they arrive at warning level so they stay separable.

## See also

- [Logging](logging.md) for the structured access log and the `request_id` that ties it to a Sentry event.
- [Metrics](metrics.md) for connection and transfer counters.
- [Configuration](configuration.md) for every environment variable.

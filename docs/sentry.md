---
description: Send application errors to Sentry and choose which details to include.
---

# Error tracking (Sentry)

[Sentry](https://sentry.io) helps you find crashes and application errors. It is optional and off by default.

To enable it, set `GFTP_SENTRY_DSN` to your Sentry project's connection URL, then restart GoblinFTP.

## Server errors

<!-- confgen:begin env-table "Error tracking (Sentry)" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_SENTRY_DSN` | (none) | Sentry project connection URL. Empty turns off backend error reporting. |
| `GFTP_SENTRY_ENVIRONMENT` | (none) | Environment name shown in Sentry, such as production or staging. |
| `GFTP_SENTRY_RELEASE` | (none) | App version shown in Sentry. Empty uses the build version. |
| `GFTP_SENTRY_SAMPLE_RATE` | `0` | Share of requests to trace, from 0 to 1. Set to 0 to turn off tracing. Does not affect error reports. |
| `GFTP_SENTRY_ERROR_SAMPLE_RATE` | `1` | Share of errors to report, from 0 to 1. Set to 1 to report every error, or 0 to report none. |
| `GFTP_SENTRY_CAPTURE_REMOTE_ERRORS` | `false` | Also report remote-server problems, such as connection failures, timeouts, and full disks. These already appear in logs and metrics. |
| `GFTP_SENTRY_SEND_SESSION_CONTEXT` | `false` | Include the FTP username, server address, and short session ID in Sentry reports. This shares customer details with Sentry. |
<!-- confgen:end -->

The release name uses the app's build version unless you set one yourself.

### Which errors are reported

| Problem | Sent to Sentry? |
|---|---|
| App crash or internal server error | Yes |
| SFTP server key changed | Yes, as a warning |
| Remote server unavailable, timeout, TLS failure, or full disk | Only with `GFTP_SENTRY_CAPTURE_REMOTE_ERRORS=true` |
| Wrong password, blocked login, missing file, or permission denied | No; check the logs |
| Browser error forwarded to the backend | Yes |

Remote connection problems already appear in [logs](logging.md) and [metrics](metrics.md). Keep them out of Sentry unless you need to investigate them there.

### What a report includes

Reports include the request ID, API route, error code, connection type, and underlying error. Use `request_id` to find the matching log entry.

Set `GFTP_SENTRY_SEND_SESSION_CONTEXT=true` to also include the FTP username, server address, and short session ID.

### Privacy

Passwords, login links, download tokens, session cookies, and CSRF tokens are filtered from request details. Client IP addresses are not attached.

Usernames and session details are off by default. **Error messages can still contain a remote server address**, such as a hostname in a connection failure. If you cannot share that with Sentry, leave the DSN empty and use local logs.

### Tracing

Tracing shows how long requests take. To trace 10% of requests, set `GFTP_SENTRY_SAMPLE_RATE=0.1`. Set it to `0` to turn tracing off.

Error reporting has a separate setting: `GFTP_SENTRY_ERROR_SAMPLE_RATE`. Its default, `1`, sends every error. `0.1` sends about 10%, and `0` sends none.

## Browser errors

Browser errors reach Sentry through the backend when `GFTP_SENTRY_DSN` is set. You can also configure the browser SDK directly, for example to collect browser performance traces.

Browser settings are applied **when building the image**. Changing them requires a new build.

| Variable | Default | Meaning |
|---|---|---|
| `NUXT_PUBLIC_SENTRY_DSN` | (none) | Sentry connection URL for the browser SDK |
| `NUXT_PUBLIC_SENTRY_ENVIRONMENT` | (none) | Environment name for browser reports |
| `NUXT_PUBLIC_SENTRY_RELEASE` | `$VERSION` | App version; use the same version as the backend |
| `NUXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE` | `0` | Share of browser requests to trace, from 0 to 1 |

Sensitive URL values are filtered before browser reports are sent. This includes login and download tokens, passwords, session details, and remote file paths in query parameters.

### How browser errors reach Sentry

GoblinFTP avoids duplicate reports:

- With a browser DSN, the browser sends the report and shares its event ID with the backend.
- Without one, the backend forwards browser reports to Sentry.

Repeated browser errors are grouped, with a limit of 20 reports per page load. `GFTP_LOG_FRONTEND` controls backend logging of browser errors; it does not turn off the browser SDK.

### Source maps

Source maps make browser stack traces easier to read. They connect the built JavaScript back to your source files.

::: details Upload source maps when building your own image

GoblinFTP generates hidden source maps, but the Docker build removes them before packaging the app. Upload them to Sentry before the deletion step in `docker/Dockerfile`.

For a local build, add the Sentry Nuxt module to `modules` in `nuxt.config.ts` and provide your project details:

```bash
SENTRY_AUTH_TOKEN=… SENTRY_ORG=… SENTRY_PROJECT=… pnpm --filter goblinftp-frontend run generate
```

To configure the browser SDK in a Docker build:

```bash
docker build -f docker/Dockerfile \
  --build-arg VERSION=v1.2.3 \
  --build-arg NUXT_PUBLIC_SENTRY_DSN=https://… \
  --build-arg NUXT_PUBLIC_SENTRY_ENVIRONMENT=production \
  -t goblinftp .
```

The Dockerfile accepts the browser DSN, environment, and release as build arguments. Add your source-map upload step before the command that deletes `*.map` files.

If you want to use one image in several environments, leave the browser DSN empty and let each backend forward errors using its own settings.

:::

## Where to look for problems

| Question | Where to look |
|---|---|
| Did the app crash? | Sentry |
| Why can a user not connect? | [Logs](logging.md), using the error code, server, and username |
| Are connection failures increasing? | [Metrics](metrics.md) |
| Which version introduced the problem? | The release name in Sentry |

## See also

- [Logging](logging.md) for request details.
- [Metrics](metrics.md) for activity and failure trends.

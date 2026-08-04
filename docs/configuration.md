# Configuration

Configuration resolves in two layers, evaluated once at process start:

1. **Environment variables** (below) for secrets, deployment toggles, and per-environment values. They always win.
2. **`settings.json`** for runtime UI, editor, connection, and access defaults. Where an env var and a `settings.json` field overlap (page title, branding, FTPS verification), the env var overrides.

Both are read once at startup by `config.Load`. There is no hot reload, so a change to either requires a restart. Invalid values in most variables abort startup with a specific error rather than being silently coerced; the exceptions are called out below.

Larger subsystems have dedicated pages: [Logging](logging.md), [Metrics](metrics.md), [S3 chunk staging](s3-staging.md), [Theming](theming.md), and [SSO login links](../examples/sso/README.md). The complete annotated variable list is [`.env.example`](../.env.example).

## Environment variables

Notation: "(none)" means unset by default; "fails startup" means `config.Load` returns an error and the process exits non-zero.

### Server

| Variable | Default | Description |
|---|---|---|
| `GFTP_PORT` | `8080` | Listen port for the Go backend (Caddy proxies to it inside the container). Not range-validated; an unusable value fails at `Start`, not at config load. |
| `GFTP_SETTINGS_PATH` | `/app/data/settings.json` | Path to `settings.json`. Read once at startup. |
| `GFTP_PAGE_TITLE` | `GoblinFTP` | Browser tab title. When non-empty, overrides `ui.pageTitle`. |

### Secrets

| Variable | Default | Description |
|---|---|---|
| `GFTP_SESSION_SECRET` | (auto-generated) | Session cookie signing key, used verbatim as bytes. When unset, a 32-byte CSPRNG key is generated and a warning is logged; every session is then invalidated on restart. |
| `GFTP_DOWNLOAD_TOKEN_SECRET` | (auto-generated) | HMAC key for signed download tokens. Same 32-byte auto-generation and restart caveat as the session secret. |

Generate a stable value with `openssl rand -hex 32`.

### Login and sessions

| Variable | Default | Description |
|---|---|---|
| `GFTP_SESSION_TTL_SECS` | `7200` | Session lifetime in seconds. Must be a positive integer or startup fails. |
| `GFTP_LOGIN_MAX_ATTEMPTS` | `5` | Failed connect attempts tolerated per `host:username` key before the cooldown rejects further attempts. Positive integer. |
| `GFTP_LOGIN_COOLDOWN_SECS` | `300` | Sliding cooldown in seconds, keyed on `host:username`. Each failed attempt extends the window; a successful connect clears the counter. Positive integer. Throttle state is in-memory and reset on restart. |
| `GFTP_DISABLE_LOGIN_FORM` | `false` | Hide the manual login form (SSO-only deployments). Enabled only by the exact string `true`. |
| `GFTP_LOGIN_DISABLED_REDIRECT` | (none) | Optional URL to redirect users who hit the disabled login form. |

### Uploads and chunking

| Variable | Default | Description |
|---|---|---|
| `GFTP_CHUNK_SIZE` | `5242880` | Upload chunk size in bytes (5 MiB). Parsed as a base-10 int64; must be greater than 0 or startup fails. |
| `GFTP_MAX_CONCURRENT_UPLOADS` | `1` | Maximum parallel chunk uploads per session. Positive integer. A single control connection serializes transfers under a per-session lock, so values above 1 mostly queue on that lock rather than adding throughput. |

### Connections and TLS

| Variable | Default | Description |
|---|---|---|
| `GFTP_FTP_TLS_INSECURE_SKIP_VERIFY` | `false` | Skip FTPS (explicit TLS) certificate verification, for self-signed or internal servers only. Enabled only by the exact string `true`. Overrides `connection.ftpTLSInsecureSkipVerify`. Applies to the `ftps` protocol; it does not affect SFTP. |

SFTP host keys are verified separately against `<GFTP_DATA_DIR>/known_hosts` on a trust-on-first-use basis: the first connection to a host pins its key, and later key changes are rejected as a mismatch.

### White-label branding

Empty values leave the `settings.json` value (or built-in default) in place.

| Variable | Default | Description |
|---|---|---|
| `GFTP_APP_NAME` | `GoblinFTP` | App name in header, login card, title, and footer. Falls back to `GoblinFTP` if resolved empty. |
| `GFTP_LOGO_URL` / `GFTP_FAVICON_URL` | (none) | Logo and favicon image URLs. A set logo hides the app-name text; the favicon falls back to the logo. |
| `GFTP_LOGO_DARK_URL` | (none) | Dark-mode logo (a light wordmark), swapped in client-side under dark mode. |
| `GFTP_PRIMARY_COLOR` | (none) | Accent color as hex matching `^#([0-9a-fA-F]{3}\|[0-9a-fA-F]{6})$` (`#RGB` or `#RRGGBB`). Recolors the theme at runtime. An invalid value fails startup. |
| `GFTP_PRIMARY_TEXT_COLOR` | (none) | Button/primary text color, same hex constraint. Pair a light accent with dark text for legible buttons. |
| `GFTP_HIDE_ATTRIBUTION` | `false` | Hide the app-name/version footer. Enabled only by the exact string `true`. |

These override the matching `branding` block in `settings.json`. For per-tenant themes, see [Theming](theming.md).

### Iframe embedding

Drops GoblinFTP into a hosting control panel as a pre-authenticated iframe. Combine with
[SSO login links](#sso-login-links) so the panel establishes the session.

| Variable | Default | Description |
|---|---|---|
| `GFTP_FRAME_ANCESTORS` | (none, framing denied) | Space-separated origins allowed to embed GoblinFTP, e.g. `https://panel.example.com https://eu.panel.example.com`. Each entry must be `scheme://host[:port]` with no path. A leftmost-label wildcard (`https://*.example.com`) is allowed. Commas, bare hosts, `*`, and non-loopback `http://` are rejected at startup. |
| `GFTP_EMBED_CHROMELESS` | `auto` | `auto` hides branding chrome only when the page is framed, `on` always hides it, `off` never does. Overrides `embed.chromeless` in `settings.json`. |

`GFTP_FRAME_ANCESTORS` is env-only by design. Caddy serves `index.html`, which is the document
`frame-ancestors` actually applies to, and Caddy cannot read `settings.json`. A value placed there
would put the header on `/api/*` and leave the document without one, so framing would fail silently
while the config looked correct.

Points to know before enabling it:

1. **Unconfigured now means denied.** With no allowlist GoblinFTP sends `frame-ancestors 'none'`
   plus `X-Frame-Options: DENY`. This is a change from earlier versions, which shipped no framing
   restriction at all and could be embedded by anyone.
2. **The session cookie changes instance-wide.** Setting an allowlist switches `gftp_session` to
   `SameSite=None; Secure; Partitioned` for every user, framed or not, because a cross-site iframe
   receives no cookie otherwise. GoblinFTP must therefore be served over HTTPS. The
   `X-CSRF-Token` header remains the CSRF defence; it is sufficient because no CORS headers are
   ever emitted, so a cross-origin page cannot read the token.
3. **A partitioned session is keyed to the embedding site.** It is not shared with a top-level tab
   on the same browser, so the user is logged in inside the panel and logged out outside it.
4. **Safari blocks third-party cookies unconditionally.** A cross-registrable-domain embed does not
   work there. The recommended topology is a shared parent domain (`panel.example.com` framing
   `files.example.com`), which sidesteps the whole class of problem.
5. **SSO tokens are single-use.** The panel must mint a fresh `/?sso=` URL on every render of the
   iframe. A browser reload of the frame re-requests its `src` and lands on `?sso_error=used`.
6. Consider `GFTP_DISABLE_LOGIN_FORM=true` for panel embeds. When the session ends, GoblinFTP then
   shows a "reopen from your control panel" message instead of a credential form the panel user
   cannot fill in.

If the frame loads but never signs in, the cause is almost always the cookie. Open DevTools,
inspect the `Set-Cookie` on the SSO response, and confirm it carries `SameSite=None; Secure;
Partitioned`. The startup log prints the active policy whenever an allowlist is configured.

### SSO login links

| Variable | Default | Description |
|---|---|---|
| `GFTP_SSO_ENABLED` | `false` | Enable one-time SSO login links. Enabled only by the exact string `true`. |
| `GFTP_SSO_SECRET` | (none) | Shared secret for SSO token validation, used as HKDF input keying material. Required when SSO is enabled, or startup fails. |

Tokens are AES-256-GCM sealed under an `HKDF-SHA256(secret, info="gftp-sso")` key, carry an `exp` timestamp, and are single-use. Replay protection is in-memory: a token becomes replayable if the backend restarts before it expires, so keep TTLs short (minutes). Generate links with `just sso-link` or the generators in [`examples/sso/`](../examples/sso/). The `-tenant <name>` flag selects a per-tenant theme (see [Theming](theming.md)).

### Error tracking (Sentry, optional)

| Variable | Default | Description |
|---|---|---|
| `GFTP_SENTRY_DSN` | (none) | Backend DSN. Empty disables backend reporting. An init failure logs a warning and does not block startup. |
| `NUXT_PUBLIC_SENTRY_DSN` | (none) | Frontend DSN, read at SPA build time (baked into the static output). |
| `GFTP_SENTRY_ENVIRONMENT` | (none) | Environment tag passed through verbatim. |
| `GFTP_SENTRY_RELEASE` | (build version) | Release tag. Defaults to `main.version` (the release tag, or `dev` for non-release builds). |
| `GFTP_SENTRY_SAMPLE_RATE` | `0` | Traces sample rate, parsed as a float. Unparseable values fall back to `0` (no error). |

### Logging, metrics, and S3

Dedicated pages own these variable groups:

- `GFTP_LOG_*`: [Logging](logging.md).
- `GFTP_METRICS_*`: [Metrics](metrics.md).
- `GFTP_S3_*`: [S3 chunk staging](s3-staging.md).

## settings.json

Read once at startup from `GFTP_SETTINGS_PATH`. Load semantics:

- **File missing:** built-in defaults are used, no error.
- **File present but unreadable** (permissions, and similar): startup fails.
- **File present but invalid JSON:** startup fails.
- **Env overrides** are applied after the file loads: `GFTP_PAGE_TITLE`, all branding variables, and `GFTP_FTP_TLS_INSECURE_SKIP_VERIFY`.

Validated fields: `connection.presetPort` must be `1-65535`; `connection.lockHost` requires a non-empty `connection.presetHost`; `branding.primaryColor` / `branding.primaryTextColor` must match the hex pattern above. Any violation fails startup.

```bash
docker run -p 8080:80 \
  -e GFTP_SESSION_SECRET="change-me" \
  -e GFTP_DOWNLOAD_TOKEN_SECRET="change-me" \
  -v ./settings.json:/app/data/settings.json:ro \
  ghcr.io/darthsoup/goblinftp
```

Key options (full schema in [`settings.example.json`](../settings.example.json)):

| Setting | Type | Description |
|---|---|---|
| `language` | string | Default UI language. Not allow-listed server-side; passed through to the SPA, which gates the actual set. Users can override it locally. |
| `ui.pageTitle` | string | Browser tab title (`GFTP_PAGE_TITLE` overrides). |
| `ui.showDotFiles` | bool | Show dotfiles by default (users can override). |
| `ui.showNavigationHistory` | bool | Show the recent-paths navigation history. |
| `ui.helpUrl` | string \| null | Optional help-link URL. |
| `editor.disabled` | bool | Disable the file editor entirely. |
| `editor.viewOnly` | bool | Open files read-only. |
| `editor.openOnCreate` | bool | Open a newly created file automatically. |
| `editor.allowedExtensions` | string[] | Editable extensions (without the dot). |
| `connection.allowedTypes` | string[] | Any subset of `["ftp","ftps","sftp"]`. |
| `connection.disableChmod` | bool | Hide the chmod UI. |
| `connection.requestTimeoutSeconds` | int | Timeout for FTP/SFTP operations. |
| `connection.presetHost` / `presetPort` | string / int | Prefill the login form (`presetPort` in `1-65535`). |
| `connection.lockHost` | bool | Make host and port read-only (requires `presetHost`). |
| `connection.passiveMode` | bool | Default for the FTP passive-mode toggle. |
| `connection.ftpTLSInsecureSkipVerify` | bool | Skip FTPS cert verification (env overrides). |
| `access.allowedClientAddresses` | string[] | IP allowlist (empty allows all). |
| `access.deniedMessage` | string \| null | Message shown to blocked clients. |
| `access.postLogoutUrl` | string \| null | Redirect target after logout. |
| `branding.*` | object | App name, logos, colors, attribution (see [Theming](theming.md)). |

## See also

- [Installation](installation.md) for Docker and manual setup.
- [System requirements](system-requirements.md).
- [Development](development.md) for the local workflow.

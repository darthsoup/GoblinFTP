# Configuration

GoblinFTP is configured entirely through environment variables, read once at startup by `config.Load` and driven by a single key registry in `backend/internal/config`. Names follow one rule: `GFTP_<SECTION>_<KEY>`. There is no hot reload, so a change requires a restart. Invalid values abort startup with a specific error naming the offending variable rather than being silently coerced. An empty variable counts as unset. Booleans accept the `strconv.ParseBool` forms (`true`, `false`, `1`, `0`, `t`, `f`, any casing). Lists are comma-separated.

Upgrading from v0.25 or earlier: several variables were renamed and the `settings.json` file was removed. Nothing is silently ignored. A stale variable aborts startup with an error naming its replacement, and a `settings.json` still present in the data directory does the same.

Larger subsystems have dedicated pages: [Logging](logging.md), [Metrics](metrics.md), [S3 chunk staging](s3-staging.md), [Theming](theming.md), [Iframe embedding](embedding.md), and [SSO login links](../examples/sso/README.md). The complete annotated variable list is [`.env.example`](../.env.example).

## Environment variables

Notation: "(none)" means unset by default; "fails startup" means `config.Load` returns an error and the process exits non-zero.

### Server

<!-- confgen:begin env-table "Server" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_PORT` | `8080` | Listen port of the Go backend, for from-source installs. Fixed at 8080 inside the container. |
| `GFTP_DATA_DIR` | `/app/data` | Writable data directory: SFTP known_hosts, local chunk staging, themes. For local dev use a relative path like data (just dev-be resolves it from the repo root). |
<!-- confgen:end -->

### UI

<!-- confgen:begin env-table "UI" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_LANGUAGE` | `en` | Default UI language. Users can override it locally; the SPA gates the actual set. |
| `GFTP_UI_PAGE_TITLE` | (none) | Browser tab title. Empty follows the branding app name. |
| `GFTP_UI_SHOW_DOT_FILES` | `false` | Show dotfiles by default (users can override). |
| `GFTP_UI_SHOW_NAVIGATION_HISTORY` | `true` | Show the recent-paths navigation history. |
<!-- confgen:end -->

### Editor

<!-- confgen:begin env-table "Editor" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_EDITOR_ALLOWED_EXTENSIONS` | `html,htm,xhtml,css,scss,sass,less,js,mjs,cjs,jsx,ts,tsx,vue,svelte,php,phtml,py,rb,go,rs,java,c,h,cpp,hpp,sh,bash,zsh,pl,lua,json,json5,xml,svg,yaml,yml,toml,ini,conf,cfg,env,properties,sql,csv,tsv,txt,md,markdown,rst,log,htaccess,htpasswd,gitignore,editorconfig,twig,ejs,hbs,mustache,liquid,erb,j2` | Editable file extensions, without the dot. |
| `GFTP_EDITOR_DISABLED` | `false` | Disable the file editor entirely. |
| `GFTP_EDITOR_VIEW_ONLY` | `false` | Open files read-only. |
<!-- confgen:end -->

### Access

<!-- confgen:begin env-table "Access" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_ACCESS_ALLOWED_CLIENT_ADDRESSES` | (none) | Client IP or CIDR allowlist; empty allows all. |
| `GFTP_ACCESS_TRUSTED_PROXIES` | (none) | CIDR ranges whose X-Forwarded-For and X-Forwarded-Proto are trusted. Set this when a reverse proxy sits in front, otherwise every client is seen as the proxy and the client allowlist cannot work. Empty trusts none. |
<!-- confgen:end -->

### Secrets

<!-- confgen:begin env-table "Secrets" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_SESSION_SECRET` | (auto-generated) | Session cookie signing key, used verbatim as bytes. |
| `GFTP_DOWNLOAD_TOKEN_SECRET` | (auto-generated) | HMAC key for signed download tokens. |
<!-- confgen:end -->

Generate a stable value with `openssl rand -hex 32`.

### Login and sessions

<!-- confgen:begin env-table "Login and sessions" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_SESSION_TTL_SECONDS` | `7200` | Session lifetime in seconds. |
| `GFTP_LOGIN_MAX_ATTEMPTS` | `5` | Failed connect attempts tolerated per host:username before the cooldown rejects further attempts. |
| `GFTP_LOGIN_COOLDOWN_SECONDS` | `300` | Sliding login cooldown in seconds, keyed on host:username. |
| `GFTP_LOGIN_FORM_DISABLED` | `false` | Disable manual logins for SSO-only deployments: the form is hidden and POST /api/auth/connect is rejected. SSO logins are unaffected. |
<!-- confgen:end -->

The login throttle state is in-memory and reset on restart; each failed attempt extends the cooldown window, a successful connect clears the counter.

### Uploads and chunking

<!-- confgen:begin env-table "Uploads and chunking" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_UPLOAD_CHUNK_SIZE` | `5242880` | Upload chunk size in bytes. |
| `GFTP_UPLOAD_MAX_CONCURRENT` | `1` | Maximum parallel chunk uploads per session. A single control connection serializes transfers, so values above 1 mostly queue. |
<!-- confgen:end -->

### Connections and TLS

<!-- confgen:begin env-table "Connections and TLS" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_CONNECTION_ALLOWED_TYPES` | `ftp,ftps,sftp` | Protocols offered on the login form; any subset of ftp, ftps, sftp. |
| `GFTP_CONNECTION_DISABLE_CHMOD` | `false` | Hide the chmod UI. |
| `GFTP_CONNECTION_PRESET_HOST` | (none) | Prefill the login form host. |
| `GFTP_CONNECTION_PRESET_PORT` | (none) | Prefill the login form port. |
| `GFTP_CONNECTION_LOCK_HOST` | `false` | Make host and port read-only on the login form; requires a preset host. |
| `GFTP_CONNECTION_PASSIVE_MODE` | `true` | Default for the FTP passive-mode toggle. |
| `GFTP_CONNECTION_FTP_TLS_INSECURE_SKIP_VERIFY` | `false` | Skip FTPS (explicit TLS) certificate verification, for self-signed or internal servers only. Does not affect SFTP. |
<!-- confgen:end -->

SFTP host keys are verified separately against `<GFTP_DATA_DIR>/known_hosts` on a trust-on-first-use basis: the first connection to a host pins its key, and later key changes are rejected as a mismatch.

### White-label branding

<!-- confgen:begin env-table "White-label branding" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_BRANDING_APP_NAME` | `GoblinFTP` | App name in header, login card, title, and footer. Falls back to GoblinFTP when empty. |
| `GFTP_BRANDING_LOGO_URL` | (none) | Logo image URL. A set logo hides the app-name text. |
| `GFTP_BRANDING_LOGO_DARK_URL` | (none) | Dark-mode logo (a light wordmark), swapped in client-side under dark mode. |
| `GFTP_BRANDING_FAVICON_URL` | (none) | Favicon image URL; falls back to the logo. |
| `GFTP_BRANDING_PRIMARY_COLOR` | (none) | Accent color as #RGB or #RRGGBB; recolors the theme at runtime. |
| `GFTP_BRANDING_PRIMARY_TEXT_COLOR` | (none) | Button/primary text color as hex; pair a light accent with dark text. |
| `GFTP_BRANDING_HIDE_ATTRIBUTION` | `false` | Hide the app-name/version footer attribution. |
<!-- confgen:end -->

For per-tenant themes, see [Theming](theming.md).

### Iframe embedding

Drops GoblinFTP into a hosting control panel as a pre-authenticated iframe. Combine with
[SSO login links](#sso-login-links) so the panel establishes the session. Full guide:
[Iframe embedding](embedding.md).

<!-- confgen:begin env-table "Iframe embedding" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_FRAME_ANCESTORS` | (none) | Space-separated origins allowed to embed GoblinFTP in an iframe. Unset denies framing. Also read by Caddy. |
| `GFTP_EMBED_CHROMELESS` | `auto` | auto hides branding chrome only when framed, on always, off never. |
<!-- confgen:end -->

Two points decide whether an embed works at all. **Unconfigured means denied:** with no allowlist
GoblinFTP sends `frame-ancestors 'none'` plus `X-Frame-Options: DENY`, a change from versions before
0.25.0, which shipped no framing restriction. **The session cookie changes instance-wide:** setting
an allowlist switches `gftp_session` to `SameSite=None; Secure; Partitioned` for every user, framed
or not, so GoblinFTP must be served over HTTPS.

`GFTP_FRAME_ANCESTORS` is read by both the Go backend and Caddy, which serves the framed document.
Full setup, accepted values, browser support, and troubleshooting: [Iframe embedding](embedding.md).

### SSO login links

<!-- confgen:begin env-table "SSO login links" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_SSO_ENABLED` | `false` | Enable one-time SSO login links. |
| `GFTP_SSO_SECRET` | (none) | Shared secret for SSO token validation. Required when SSO is enabled. |
<!-- confgen:end -->

Tokens are AES-256-GCM sealed under an `HKDF-SHA256(secret, info="gftp-sso")` key, carry an `exp` timestamp, and are single-use. Replay protection is in-memory: a token becomes replayable if the backend restarts before it expires, so keep TTLs short (minutes). Generate links with `just sso-link` or the generators in [`examples/sso/`](../examples/sso/). The `-tenant <name>` flag selects a per-tenant theme (see [Theming](theming.md)).

### Logging, metrics, error tracking, and S3

Dedicated pages own these variable groups:

- `GFTP_LOG_*`: [Logging](logging.md).
- `GFTP_METRICS_*`: [Metrics](metrics.md).
- `GFTP_SENTRY_*` and `NUXT_PUBLIC_SENTRY_*`: [Error tracking](sentry.md).
- `GFTP_S3_*`: [S3 chunk staging](s3-staging.md).

## See also

- [Installation](installation.md) for Docker and manual setup.
- [System requirements](system-requirements.md).
- [Development](development.md) for the local workflow.

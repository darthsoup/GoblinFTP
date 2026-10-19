---
description: Change login, upload, connection, and branding settings for GoblinFTP.
---

# Configuration

Use the settings below to change how GoblinFTP works. Add them to your Docker setup or `.env` file, then restart GoblinFTP. See [Installation](installation.md) for an example.

Start with [`.env.example`](https://github.com/darthsoup/goblinftp/blob/main/.env.example) for a complete list of settings.

## Environment variables

Use `true` or `false` to turn options on or off. Separate list values with commas. "(none)" means the setting is empty by default. If a value is invalid, GoblinFTP stops at startup and tells you which setting to fix.

### Server

<!-- confgen:begin env-table "Server" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_BIND_ADDRESS` | (none) | IP address to listen on. Empty allows all addresses. Docker sets this to 127.0.0.1. |
| `GFTP_PORT` | `8080` | Backend port for installations without Docker. In Docker, the backend uses 8081 and the app uses 8080. |
| `GFTP_DATA_DIR` | `/app/data` | Folder for trusted SFTP keys, temporary uploads, and themes. Use data for local development. |
<!-- confgen:end -->

### UI

<!-- confgen:begin env-table "UI" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_LANGUAGE` | `en` | Default language. Users can choose another language in settings. |
| `GFTP_UI_PAGE_TITLE` | (none) | Browser tab title. Empty uses the app name. |
| `GFTP_UI_SHOW_DOT_FILES` | `false` | Show files whose names start with a dot. Users can change this in settings. |
| `GFTP_UI_SHOW_NAVIGATION_HISTORY` | `true` | Show recently visited folders. |
<!-- confgen:end -->

### Editor

<!-- confgen:begin env-table "Editor" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_EDITOR_ALLOWED_EXTENSIONS` | `html,htm,xhtml,css,scss,sass,less,js,mjs,cjs,jsx,ts,tsx,vue,svelte,php,phtml,py,rb,go,rs,java,c,h,cpp,hpp,sh,bash,zsh,pl,lua,json,json5,xml,svg,yaml,yml,toml,ini,conf,cfg,env,properties,sql,csv,tsv,txt,md,markdown,rst,log,htaccess,htpasswd,gitignore,editorconfig,twig,ejs,hbs,mustache,liquid,erb,j2` | File types users can edit. List extensions without the leading dot. |
| `GFTP_EDITOR_DISABLED` | `false` | Turn off the file editor. |
| `GFTP_EDITOR_VIEW_ONLY` | `false` | Allow users to view files in the editor, but not save changes. |
<!-- confgen:end -->

### Access

<!-- confgen:begin env-table "Access" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_ACCESS_ALLOWED_CLIENT_ADDRESSES` | (none) | IP addresses or network ranges allowed to use GoblinFTP. Empty allows everyone. |
| `GFTP_ACCESS_TRUSTED_PROXIES` | (none) | Proxy IP addresses or network ranges to trust for client addresses and HTTPS detection. In Docker, list only your external proxies; Caddy is already trusted. Empty trusts none for installations without Docker. |
<!-- confgen:end -->

### Secrets

<!-- confgen:begin env-table "Secrets" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_SESSION_SECRET` | (none) | Unused. Setting this does not keep users signed in after a restart. |
| `GFTP_DOWNLOAD_TOKEN_SECRET` | (auto-generated) | Secret used to sign download links. Generated automatically when empty. |
<!-- confgen:end -->

To create your own download-link secret, run `openssl rand -hex 32`. Restarting GoblinFTP still invalidates existing download links.

### Login and sessions

<!-- confgen:begin env-table "Login and sessions" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_SESSION_TTL_SECONDS` | `7200` | How long a login session lasts, in seconds. |
| `GFTP_LOGIN_MAX_ATTEMPTS` | `5` | Failed logins allowed for each server and username before further attempts are temporarily blocked. |
| `GFTP_LOGIN_COOLDOWN_SECONDS` | `300` | Seconds to wait after a failed login before the block expires. Each failure restarts this timer. |
| `GFTP_LOGIN_FORM_DISABLED` | `false` | Require SSO links to log in. Hides the login form and blocks manual login requests. |
<!-- confgen:end -->

Each failed login restarts the waiting period. A successful login clears the failure count. Restarting GoblinFTP clears all login blocks.

### Uploads and chunking

<!-- confgen:begin env-table "Uploads and chunking" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_UPLOAD_CHUNK_SIZE` | `5242880` | Size of each upload part, in bytes. |
| `GFTP_UPLOAD_MAX_CONCURRENT` | `1` | Upload parts sent at once per session. Keep at 1 in most setups; the remote connection handles transfers one at a time. |
<!-- confgen:end -->

### Connections and TLS

<!-- confgen:begin env-table "Connections and TLS" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_CONNECTION_ALLOWED_TYPES` | `ftp,ftps,sftp` | Connection types available on the login form: ftp, ftps, and sftp. |
| `GFTP_CONNECTION_DISABLE_CHMOD` | `false` | Hide the option to change file permissions. |
| `GFTP_CONNECTION_PRESET_HOST` | (none) | Default server address on the login form. |
| `GFTP_CONNECTION_PRESET_PORT` | (none) | Default server port on the login form. |
| `GFTP_CONNECTION_LOCK_HOST` | `false` | Prevent changes to the server address and port. Requires a default server address. |
| `GFTP_CONNECTION_PASSIVE_MODE` | `true` | Use passive mode for FTP connections by default. |
| `GFTP_CONNECTION_FTP_TLS_INSECURE_SKIP_VERIFY` | `false` | Allow FTPS connections without checking the certificate. Use only for trusted servers with self-signed certificates. Does not affect SFTP. |
<!-- confgen:end -->

On your first SFTP connection, GoblinFTP asks you to trust the server key and saves it in `<GFTP_DATA_DIR>/known_hosts`. Later connections must use the same key. A changed key is rejected.

### White-label branding

<!-- confgen:begin env-table "White-label branding" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_BRANDING_APP_NAME` | `GoblinFTP` | App name shown in the interface. Empty uses GoblinFTP. |
| `GFTP_BRANDING_LOGO_URL` | (none) | Logo image URL. The logo replaces the app name in the header and login screen. |
| `GFTP_BRANDING_LOGO_DARK_URL` | (none) | Logo image URL to use in dark mode. |
| `GFTP_BRANDING_FAVICON_URL` | (none) | Browser tab icon URL. Empty uses the logo. |
| `GFTP_BRANDING_PRIMARY_COLOR` | (none) | Main accent color, such as #2563eb. Accepts #RGB or #RRGGBB. |
| `GFTP_BRANDING_PRIMARY_TEXT_COLOR` | (none) | Text color on primary buttons, as a hex color. Use dark text with a light accent. |
| `GFTP_BRANDING_HIDE_ATTRIBUTION` | `false` | Hide the GoblinFTP name and version in the footer. |
<!-- confgen:end -->

To use a different look for each customer, see [Branding and themes](theming.md).

### Iframe embedding

Show GoblinFTP inside your app or hosting panel. Use [SSO login links](sso.md) to sign users in automatically.

<!-- confgen:begin env-table "Iframe embedding" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_FRAME_ANCESTORS` | (none) | Website addresses allowed to embed GoblinFTP. Separate them with spaces. Empty blocks embedding. |
| `GFTP_EMBED_CHROMELESS` | `auto` | Hide extra branding: auto when embedded, on always, off never. |
<!-- confgen:end -->

Embedding is disabled until you set the allowed origins. When enabled, serve GoblinFTP over HTTPS. See [Iframe embedding](embedding.md) for setup and browser compatibility.

### SSO login links

<!-- confgen:begin env-table "SSO login links" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_SSO_ENABLED` | `false` | Allow users to log in through one-time links. |
| `GFTP_SSO_SECRET` | (none) | Secret shared with your login-link generator. Required when SSO is enabled. |
<!-- confgen:end -->

Generate one-time login links with `just sso-link` or the Node.js and PHP examples. See [SSO login links](sso.md) for setup, options, and token details.

### Logging, metrics, error tracking, and S3

These settings have their own guides:

- `GFTP_LOG_*`: [Logging](logging.md).
- `GFTP_METRICS_*`: [Metrics](metrics.md).
- `GFTP_SENTRY_*` and `NUXT_PUBLIC_SENTRY_*`: [Error tracking](sentry.md).
- `GFTP_S3_*`: [S3 chunk staging](s3-staging.md).

## See also

- [Installation](installation.md) for Docker and manual setup.
- [System requirements](system-requirements.md).
- [Development](development.md) for the local workflow.

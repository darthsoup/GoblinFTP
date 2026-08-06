# White-label theming (multi-tenant)

GoblinFTP's UI is fully driven by CSS custom properties, so a **tenant** can be
re-skinned by dropping a small stylesheet + logo on disk - no rebuild. A request's
tenant is resolved from the **SSO link** and/or the **host/subdomain**, and the
matching theme is loaded at runtime.

```
<dataDir>/themes/<tenant>/
├── config.css     # required - existence of this file is what defines the tenant
├── logo.png       # optional - or logo.svg / logo.webp / logo.jpg
├── logo-dark.png  # optional - dark-mode logo, swapped in automatically
├── favicon.ico    # optional - or favicon.png / favicon.svg
└── theme.json     # optional - maps custom domains to this tenant
```

`<dataDir>` is the container's fixed data volume, `/app/data`. Files are served read-only at
`/themes/<tenant>/<file>` by the Go backend (in the container, Caddy proxies `/themes/*` to it),
with the tenant slug and filename allowlisted server-side (path traversal is rejected).

## 1. Install a tenant

```bash
cp -r examples/themes/example-tenant  /app/data/themes/acme
# edit /app/data/themes/acme/config.css and drop in a logo
```

`<tenant>` must match `^[a-z0-9][a-z0-9_-]{0,63}$`.

## 2. Select the tenant

**Via SSO** (primary): embed it in the one-time link -

```bash
gftp-sso-link -tenant acme -host ftp.example.com -username alice -password s3cret
```

**Via a custom domain** (themes the login screen on first paint, before any token):
list the full domains in `theme.json` and keep the slug clean -

```json
{ "hosts": ["acme.example.com", "files.acme.com", "portal.acme.io"] }
```

Serve GoblinFTP at any of those hosts - a subdomain of your instance or the
customer's own vanity domain - and it resolves to tenant `acme`. Hosts match
exactly (case-insensitive, port ignored), never touch the filesystem path, and
edits take effect within ~15s. A host should map to one tenant; SSO wins when both
apply.

A missing/invalid tenant or theme **fails soft** to default GoblinFTP branding - login
still works, no error shown.

## 3. What `config.css` may override

Redefine **color** tokens under both `:root, .light { … }` and `.dark { … }`
(see [`example-tenant/config.css`](example-tenant/config.css)):

- **Accent:** `--ui-primary` - drive this **directly**; leave the global
  `branding.primaryColor` unset (the runtime accent injector is skipped when a
  tenant stylesheet is present).
- **Primary button text:** `--gftp-primary-text` - text on primary buttons/badges.
  Set it to a dark color when your accent is light (see the contrast note below).
- **Surfaces:** `--ui-bg`, `--ui-bg-muted`, `--ui-bg-elevated`, `--ui-bg-accented`, `--ui-bg-inverted`
- **Borders:** `--ui-border`, `--ui-border-muted`, `--ui-border-accented`, `--ui-border-inverted`
- **Text:** `--ui-text`, `--ui-text-dimmed`, `--ui-text-muted`, `--ui-text-toned`, `--ui-text-highlighted`, `--ui-text-inverted`
- **Extras:** `--gftp-popover`, `--gftp-popover-ring`, `--gftp-editor-bg`,
  `--gftp-scrollbar-thumb`, `--gftp-scrollbar-thumb-hover`, `--gftp-selection-bg`,
  `--gftp-selection-fg`, `--gftp-input-bg`, `--gftp-input-placeholder`, `--gftp-atmosphere`

**Leave structural tokens alone** (documented, not enforced): `--ui-radius`,
`--ui-header-height`, `--radius-*`, `--font-sans`, `--font-mono`.

> Contrast note: primary-button text uses `--gftp-primary-text` (falling back to
> `--ui-text-inverted`). If your accent is light (yellow, lime…), set
> `--gftp-primary-text` to a dark color so button text stays legible. Don't override
> `--ui-text-inverted` for this - it's shared with inverted surfaces (tooltips) and
> would make their text unreadable.

## 4. Logo

Rendered in the header (~28px tall) and login card (~44px tall), width-flexible with
`object-contain` - a **transparent PNG / SVG / WebP** wordmark or icon both work.
When a logo is set the app-name text is hidden, so the logo stands alone.

Drop a **`logo-dark.*`** beside `logo.*` for a dark-mode variant (light wordmark) -
it's swapped in automatically in dark mode, so a dark-ink logo won't vanish on the
dark canvas. `favicon.*` works the same way; if omitted, the logo doubles as the favicon.

## Security

`themes/` holds **public branding only** - any client can fetch another tenant's
`config.css`/`logo`. Never place secrets there. Admin-authored CSS is trusted
(it can pull external `url()` assets and restyle the UI), so only place theme
files you control.

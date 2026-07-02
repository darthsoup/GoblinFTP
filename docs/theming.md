# Theming

GoblinFTP supports two levels of white-labeling:

1. **Global branding** — one look for the whole instance, set with environment
   variables (`GFTP_APP_NAME`, `GFTP_LOGO_URL`, `GFTP_PRIMARY_COLOR`, …). See
   [Configuration → white-label variables](configuration.md).
2. **Per-tenant themes** — different looks for different users of the *same*
   instance, each selected at runtime by the SSO link or the host/subdomain.
   That's what this page covers.

Because the entire UI is driven by CSS custom properties, a tenant theme is just
a small stylesheet that redefines a curated set of color tokens — no rebuild, no
persistent store. Drop a folder on disk and it's live.

## How it works

```
<dataDir>/themes/<tenant>/
├── config.css     # required — its existence is what defines the tenant
├── logo.png       # optional — or logo.svg / logo.webp / logo.jpg
├── logo-dark.png  # optional — dark-mode logo, swapped in automatically
├── favicon.ico    # optional — or favicon.png / favicon.svg
└── theme.json     # optional — maps custom domains to this tenant (see § 2)
```

`<dataDir>` is the container's fixed data volume, `/app/data`. On each `GET /api/system/vars`,
the backend resolves the request's tenant and, if a theme exists, tells the SPA to
inject `<link rel="stylesheet" href="/themes/<tenant>/config.css">` and repoint the
logo/favicon. Assets are served read-only at `/themes/<tenant>/<file>`; the tenant
slug and filename are allowlisted server-side and path traversal is rejected.

**Tenant is resolved in this order:**

1. **SSO session** — the `tenant` field baked into the one-time SSO token.
2. **Custom domain** — the request host is listed in a tenant's `theme.json`.

A missing or invalid tenant (or a tenant with no `config.css`) **fails soft** to
the default GoblinFTP branding — login still works and no error is shown, so the
mechanism never reveals which tenants exist. Because the response now depends on
the cookie/host, `/api/system/vars` is sent with `Cache-Control: private, no-store`
and `Vary: Cookie`.

## 1. Install a tenant

Copy the shipped example and edit it:

```bash
cp -r examples/themes/example-tenant  /app/data/themes/acme
# then edit /app/data/themes/acme/config.css and drop in a logo
```

The tenant slug must match `^[a-z0-9][a-z0-9_-]{0,63}$` (lowercase letters,
digits, `-`, `_`).

## 2. Select the tenant

### Via SSO (primary)

Embed the tenant in the one-time login link (requires `GFTP_SSO_ENABLED=true`):

```bash
just sso-link -tenant acme -host ftp.example.com -username alice -password s3cret
```

The field is optional and part of the encrypted, tamper-proof token — see
[`examples/sso/`](../examples/sso/) for the token format and the Node/PHP generators.

The custom-domain path resolves the tenant **before any token**, so the login
screen itself is themed on first paint. When an SSO tenant and a host tenant both
apply, **SSO wins**.

### Via a custom domain (`theme.json`)

Keep the tenant slug clean and map one or more full domains to it with an optional
`theme.json` in the theme folder:

```json
{ "hosts": ["acme.example.com", "files.acme.com", "portal.acme.io"] }
```

Serve GoblinFTP at any of those hosts — a subdomain of your instance or the
customer's own vanity domain — and it resolves to that tenant. Hosts are matched
exactly (case-insensitive, port ignored) and never touch the filesystem path, so
slugs stay `^[a-z0-9][a-z0-9_-]{0,63}$`. A host should map to a single tenant; on a
collision the alphabetically-first tenant wins. Changes to `theme.json` take effect
within ~15 seconds (the host index is cached).

## 3. Write `config.css`

Redefine **color** tokens under the same selectors the base theme uses —
`:root, .light { … }` for light mode and `.dark { … }` for dark. The file is
injected *after* the bundled stylesheet, so equal-specificity `:root` rules win by
source order.

> **Set each token you override in _both_ blocks** (or leave the default). Because
> `:root` always matches and your sheet loads last, a token set only under
> `:root, .light` also applies in dark mode — it beats the base `.dark` rule — so
> dark mode would keep the light value (e.g. dark `--ui-text-highlighted` staying
> dark on the dark canvas). If a token's default is fine, don't override it at all.

| Group | Tokens you may override |
|---|---|
| **Accent** | `--ui-primary`, `--gftp-primary-text` (text on primary buttons/badges) |
| **Surfaces** | `--ui-bg`, `--ui-bg-muted`, `--ui-bg-elevated`, `--ui-bg-accented`, `--ui-bg-inverted` |
| **Borders** | `--ui-border`, `--ui-border-muted`, `--ui-border-accented`, `--ui-border-inverted` |
| **Text** | `--ui-text`, `--ui-text-dimmed`, `--ui-text-muted`, `--ui-text-toned`, `--ui-text-highlighted`, `--ui-text-inverted` |
| **Extras** | `--gftp-popover`, `--gftp-popover-ring`, `--gftp-editor-bg`, `--gftp-scrollbar-thumb`, `--gftp-scrollbar-thumb-hover`, `--gftp-selection-bg`, `--gftp-selection-fg`, `--gftp-input-bg`, `--gftp-input-placeholder`, `--gftp-atmosphere` |

**Leave structural tokens alone** (not enforced, but unsupported):
`--ui-radius`, `--ui-header-height`, `--radius-*`, `--font-sans`, `--font-mono`.

> **Accent:** drive `--ui-primary` **directly** and leave the global
> `GFTP_PRIMARY_COLOR` / `branding.primaryColor` unset. When a tenant stylesheet is
> present, the runtime accent injector is skipped — otherwise its inline
> `--color-goblin-*` overrides would beat your `:root` rules.

> **Contrast:** primary-button text uses `--gftp-primary-text` (falling back to
> `--ui-text-inverted`). If your accent is light (yellow, lime…), set
> `--gftp-primary-text` to a dark color so button text stays legible. Use this
> rather than `--ui-text-inverted` — that token is *also* the text on inverted
> surfaces (tooltips), so overriding it there would make those unreadable.

### Sample

```css
:root,
.light {
  --ui-primary: #2563eb;          /* brand accent (pairs with white button text) */

  --ui-bg: #f6f8fc;               /* canvas */
  --ui-bg-muted: #eef2f9;         /* nav strips / table header tint */
  --ui-bg-elevated: #ffffff;      /* cards & panels */
  --ui-bg-accented: #e2e8f4;      /* hover surfaces */

  --ui-border: #dbe3f0;
  --ui-text-highlighted: #0b1220;

  --gftp-editor-bg: #eef2f9;
  --gftp-input-bg: #f6f8fc;
  --gftp-selection-bg: color-mix(in oklab, #2563eb 25%, transparent);
  --gftp-selection-fg: #0b1220;
}

.dark {
  --ui-primary: #60a5fa;          /* lighter accent for the dark canvas */

  --ui-bg: #0b1220;
  --ui-bg-muted: #131c2e;
  --ui-bg-elevated: #111a2b;
  --ui-bg-accented: #1e293b;

  --ui-border: #1e293b;

  --gftp-editor-bg: #080d16;
  --gftp-input-bg: #1e293b;
  --gftp-selection-bg: color-mix(in oklab, #2563eb 30%, transparent);
  --gftp-selection-fg: #dbeafe;
}
```

The full, commented version lives at
[`examples/themes/example-tenant/config.css`](../examples/themes/example-tenant/config.css).

## 4. Logo & favicon

The logo renders in the header (~28px tall) and on the login card (~44px tall),
width-flexible with `object-contain` — a wordmark is fine. When a logo is set the
app-name text is hidden, so the logo alone represents the brand. A tenant's
`logo.*` takes precedence over a global `GFTP_LOGO_URL`.

**Dark mode:** drop a `logo-dark.*` next to `logo.*` and it's swapped in
automatically whenever the UI is in dark mode — otherwise a light-mode wordmark
(dark ink) would vanish on the dark canvas. Provide a version with a light
wordmark. `favicon.*` works like `logo.*`; if omitted, the logo doubles as the
favicon.

## Security

`themes/` holds **public branding only** — any client can fetch another tenant's
`config.css` or `logo`, so never place secrets there. Admin-authored CSS is trusted
(it can pull external `url()` assets and restyle the UI), so only place theme files
you control.

## See also

- [`examples/themes/`](../examples/themes/) — the ready-to-copy example tenant.
- [Configuration](configuration.md) — global (single-look) white-label variables and SSO.

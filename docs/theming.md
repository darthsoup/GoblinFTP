---
description: Add your logo and colors, or give each customer their own theme.
---

# Branding and themes

You can change the app name, logo, and colors for everyone, or give each customer a different look.

For one shared look, use the [branding settings](configuration.md#white-label-branding). For different looks, create a theme for each customer. These customer themes are called **tenant themes**.

## 1. Add a theme

Copy the [example theme](https://github.com/darthsoup/goblinftp/tree/main/examples/themes/example-tenant/) into your data folder. The example below uses `/app/data`, the folder inside the container. If you edit files on the host, use the folder you mounted there.

```bash
cp -r examples/themes/example-tenant /app/data/themes/acme
```

Choose a name with 1 to 64 lowercase letters, numbers, hyphens, or underscores. It must start with a letter or number.

A theme folder looks like this:

```text
/app/data/themes/acme/
├── config.css
├── logo.png
├── logo-dark.png
├── favicon.ico
└── theme.json
```

Only `config.css` is required. The other files are optional. Logos can also be SVG, WebP, or JPEG files. Favicons can also be PNG or SVG files.

## 2. Choose when to use it

### With a login link

Add the theme name when creating a [login link](sso.md):

```bash
just sso-link -tenant acme -host ftp.example.com -username alice
```

Enable SSO and set the shared secret and password as described in the SSO guide. The theme is included in the encrypted link.

### With a custom domain

To choose a theme by domain, add a `theme.json` file to its folder:

```json
{ "hosts": ["acme.example.com", "files.acme.com"] }
```

Point these domains at GoblinFTP. Each domain shows this theme, including on the login screen. Changes take effect within about 15 seconds.

Use full hostnames without a scheme or path. Matching ignores capitalization and port numbers. Give each hostname to one theme. If several themes list it, GoblinFTP uses the first theme in alphabetical order.

If a login link selects a different theme, the link takes priority. A missing or invalid theme falls back to GoblinFTP's default look, and login still works.

## 3. Set your colors

Set colors for both light and dark mode. For example:

```css
:root,
.light {
  --ui-primary: #2563eb;
  --gftp-primary-text: #ffffff;
  --ui-bg: #f6f8fc;
  --ui-text: #334155;
}

.dark {
  --ui-primary: #60a5fa;
  --gftp-primary-text: #0b1220;
  --ui-bg: #0b1220;
  --ui-text: #dbeafe;
}
```

For every color you change, set a value in **both** blocks. Otherwise, a light-mode color can appear in dark mode too. Leave out colors you do not want to change.

Use `--ui-primary` for the accent and `--gftp-primary-text` for text on primary buttons. Pair light buttons with dark text so labels stay readable. A tenant theme controls its own accent; it does not use `GFTP_BRANDING_PRIMARY_COLOR`.

### Available colors

These CSS variables control the app's colors:

| Area | Variables |
|---|---|
| Accent | `--ui-primary`, `--gftp-primary-text` |
| Backgrounds | `--ui-bg`, `--ui-bg-muted`, `--ui-bg-elevated`, `--ui-bg-accented`, `--ui-bg-inverted` |
| Borders | `--ui-border`, `--ui-border-muted`, `--ui-border-accented`, `--ui-border-inverted` |
| Text | `--ui-text`, `--ui-text-dimmed`, `--ui-text-muted`, `--ui-text-toned`, `--ui-text-highlighted`, `--ui-text-inverted` |
| Other colors | `--gftp-popover`, `--gftp-popover-ring`, `--gftp-editor-bg`, `--gftp-scrollbar-thumb`, `--gftp-scrollbar-thumb-hover`, `--gftp-selection-bg`, `--gftp-selection-fg`, `--gftp-input-bg`, `--gftp-input-placeholder`, `--gftp-atmosphere` |

For more examples, see the [sample stylesheet](https://github.com/darthsoup/goblinftp/blob/main/examples/themes/example-tenant/config.css). Changes to fonts, layout sizes, and corner radii are not supported.

## 4. Add your logo

Add `logo.*` to replace the app name with your logo. It appears in the header and on the login screen. Wide logos work too. A tenant logo takes priority over the global logo setting.

Add `logo-dark.*` if your logo needs a lighter version for dark mode. Add `favicon.*` for the browser tab icon. Without a favicon, GoblinFTP uses the logo.

## Security

Theme files are public. Never put passwords, tokens, or other secrets in the theme folder. Only install stylesheets you trust: CSS can change the interface and load external images.

## See also

- [Configuration](configuration.md#white-label-branding) for shared branding settings.
- [SSO login links](sso.md) for selecting a theme during login.

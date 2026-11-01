---
description: Show GoblinFTP inside your app or hosting panel and offer users one-time login links.
---

# Iframe embedding

You can show GoblinFTP inside your app or hosting panel using an iframe. A [one-time login link](sso.md) lets the user confirm the target before signing in.

GoblinFTP blocks embedding by default. You must allow your panel's website address and serve GoblinFTP over HTTPS.

## Quick start

Use related domains if possible, such as `panel.example.com` and `files.example.com`. This avoids many browser cookie restrictions.

Add these settings to your `.env` file:

```bash
GFTP_FRAME_ANCESTORS=https://panel.example.com
GFTP_SSO_ENABLED=true
GFTP_SSO_SECRET=replace-with-at-least-32-random-bytes
GFTP_LOGIN_FORM_DISABLED=true
```

Use the same random secret in your app's [link generator](sso.md#setup). Load the `.env` file as shown in [Installation](installation.md#configuration), then restart GoblinFTP.

Create a fresh link each time your panel displays the iframe. For example, using the PHP generator:

```php
$url = gftp_sso_link($secret, 'https://files.example.com', [
    'protocol' => 'sftp',
    'host' => 'sftp.example.com',
    'username' => $customer->ftpUser,
    'password' => $customer->ftpPassword,
]);
printf('<iframe src="%s" style="width:100%%;height:80vh;border:0"></iframe>', htmlspecialchars($url));
```

## Variables

<!-- confgen:begin env-table "Iframe embedding" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_FRAME_ANCESTORS` | (none) | Website addresses allowed to embed GoblinFTP. Separate them with spaces. Empty blocks embedding. |
| `GFTP_EMBED_CHROMELESS` | `auto` | Hide extra branding: auto when embedded, on always, off never. |
<!-- confgen:end -->

Enter website addresses as `scheme://host[:port]`, without a path or trailing slash. Separate several addresses with spaces. GoblinFTP checks them at startup and reports any invalid entry.

### Accepted and rejected values

| Value | Result |
|---|---|
| `https://panel.example.com` | Allowed |
| `https://panel.example.com:8443` | Allowed, with a specific port |
| `https://panel:8443` | Allowed, for an internal service name |
| `https://*.customers.example.com` | Allowed, for all subdomains |
| `http://localhost:3000` | Allowed for local development |
| `https://a.example.com https://b.example.com` | Allowed, two websites |
| `*` or `https://*` | Rejected: too broad |
| `panel.example.com` | Rejected: missing `https://` |
| `https://panel.example.com/embed` | Rejected: contains a path |
| `https://panel.example.com/` | Rejected: remove the trailing slash |
| `https://a.example.com,https://b.example.com` | Rejected: use spaces, not commas |
| `http://panel.example.com` | Rejected: HTTPS is required |
| `https://*.com` | Rejected: wildcard is too broad |
| `'self'` | Rejected: enter the website address |

## How framing is allowed

The browser checks whether your panel is allowed to display GoblinFTP. In Docker, the app handles the required headers for you.

If you use your own web server, add the same framing policy to the HTML page it serves. Allowing embedding on API responses alone is not enough. See the [container's Caddy configuration](https://github.com/darthsoup/goblinftp/blob/main/docker/Caddyfile) for an example.

## The session cookie

Allowing embedding changes the login cookie for all users. HTTPS is required even for people who open GoblinFTP directly.

When the panel and GoblinFTP use different parent domains, a login inside the panel can be separate from a login in a normal browser tab.

::: details Cookie and header details

| Cookie setting | Default | With embedding enabled |
|---|---|---|
| `SameSite` | `Lax` | `None` |
| `Secure` | On with HTTPS | Always on |
| `Partitioned` | Off | On |
| `HttpOnly`, `Path` | `HttpOnly`, `/` | Unchanged |

The HTML page and API responses use `Content-Security-Policy: frame-ancestors` with your allowed addresses. Without an allowlist, framing is denied and `X-Frame-Options: DENY` is also sent.

CSRF tokens are still required for requests that change files or settings.

:::

## Pre-authenticating with SSO

- Create a new link for each page load. Reloading an old iframe URL reuses the link and fails.
- Keep link expiry to a few minutes.
- Set `GFTP_LOGIN_FORM_DISABLED=true` if users should only enter through your panel. Signed-out visitors then see a 404 instead of the login form.
- Login sessions last two hours by default (`GFTP_SESSION_TTL_SECONDS`). When a session expires, reopen the frame with a fresh link.
- Add a `tenant` to the link to choose a [customer theme](theming.md).

## Chromeless mode

GoblinFTP hides repeated branding when it appears inside an iframe. This includes the app name, logo, disconnect button, status bar, and attribution footer. File navigation, settings, shortcuts, and the theme switch remain available.

Choose the behavior with `GFTP_EMBED_CHROMELESS`:

- `auto`: hide extra branding only when embedded (default).
- `on`: always hide it.
- `off`: keep the full interface.

Hiding controls does not change access permissions. Use [server settings](configuration.md) and FTP/SFTP permissions to restrict what users can do.

## Browser support

Using the same parent domain, such as `panel.example.com` and `files.example.com`, gives the best compatibility.

For different parent domains:

| Browser | What to expect |
|---|---|
| Chrome, Edge | Support separate cookies for embedded sessions. |
| Firefox | Keeps embedded cookies separate by website. |
| Safari | Can block the login cookie, preventing sign-in. |

Browser privacy settings can also affect login. Test the browsers your users rely on.

## Verifying the setup

Check the headers on the page and API:

```bash
curl -sI https://files.example.com/ | grep -i content-security-policy
curl -sI https://files.example.com/healthz | grep -iE 'content-security-policy|x-frame-options'
```

Both should show `frame-ancestors` followed by your allowed panel addresses. With embedding disabled, they show `frame-ancestors 'none'`.

Open your panel and try signing in. If it fails, use the browser's Network tools to check whether the SSO response sets the `gftp_session` cookie.

## Troubleshooting

| Problem | What to check |
|---|---|
| Blank frame with a `frame-ancestors` error | Match the panel address exactly, including `https://` and any port. Check the HTML page headers too. |
| Frame returns to login | Make sure GoblinFTP uses HTTPS and the browser accepts the login cookie. |
| Works in Chrome but not Safari | Use the same parent domain for the panel and GoblinFTP. |
| Login link already used | Generate a fresh link for each page load. Do not cache iframe URLs. |
| Login link expired | Check both servers' clocks and allow enough time for the page to load. |
| Container will not start | Read the startup error for an invalid allowed address. |
| Full branding still visible | Check `GFTP_EMBED_CHROMELESS` and confirm the page is inside an iframe. |

## Security notes

Only allow websites you control. Prefer exact addresses over wildcards, which also allow forgotten or compromised subdomains.

Embedding does not change file permissions. Users have the same access as they would with a direct FTP/SFTP client.

## See also

- [SSO login links](sso.md) for generating links.
- [Branding and themes](theming.md) for customer branding.
- [Installation](installation.md) for the Docker setup.

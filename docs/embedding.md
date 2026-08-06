# Iframe embedding

GoblinFTP can be embedded in a hosting control panel as a pre-authenticated iframe. The panel mints a one-time [SSO link](../examples/sso/README.md), points an `<iframe>` at it, and the customer gets a file manager inside the panel without a second login.

Two things have to be configured for that to work: the origins allowed to frame GoblinFTP, and a session cookie the browser will actually send inside a frame. Both follow from one variable, `GFTP_FRAME_ANCESTORS`.

**Framing is denied unless you configure it.** With no allowlist GoblinFTP sends `frame-ancestors 'none'` plus `X-Frame-Options: DENY` on every response. Versions before 0.25.0 shipped no framing restriction at all, so an existing embed will break on upgrade until the allowlist is set.

## Quick start

```bash
docker run -p 443:80 \
  -e GFTP_FRAME_ANCESTORS="https://panel.example.com" \
  -e GFTP_SSO_ENABLED=true \
  -e GFTP_SSO_SECRET="$(openssl rand -hex 32)" \
  -e GFTP_LOGIN_FORM_DISABLED=true \
  -e GFTP_SESSION_SECRET="$(openssl rand -hex 32)" \
  -e GFTP_DOWNLOAD_TOKEN_SECRET="$(openssl rand -hex 32)" \
  ghcr.io/darthsoup/goblinftp
```

In the panel, generate a fresh link per page render and use it as the frame source:

```php
$url = gftp_sso_link($secret, 'https://files.example.com', [
    'protocol' => 'sftp',
    'host'     => 'sftp.example.com',
    'username' => $customer->ftpUser,
    'password' => $customer->ftpPassword,
]);
printf('<iframe src="%s" style="width:100%%;height:80vh;border:0"></iframe>', htmlspecialchars($url));
```

GoblinFTP must be reachable over HTTPS. The embed cookie policy sets `Secure`, and browsers drop a `Secure` cookie on a plain HTTP origin.

## Variables

<!-- confgen:begin env-table "Iframe embedding" -->
| Variable | Default | Description |
|---|---|---|
| `GFTP_FRAME_ANCESTORS` | (none) | Space-separated origins allowed to embed GoblinFTP in an iframe. Unset denies framing. Also read by Caddy. |
| `GFTP_EMBED_CHROMELESS` | `auto` | auto hides branding chrome only when framed, on always, off never. |
<!-- confgen:end -->

Each `GFTP_FRAME_ANCESTORS` entry is `scheme://host[:port]` with no path, query, fragment, or credentials; a leftmost-label wildcard (`https://*.example.com`) is allowed. The value is validated at startup and an invalid entry aborts the process. `GFTP_EMBED_CHROMELESS` is presentation only.

See [How framing is allowed](#how-framing-is-allowed) for how the value reaches both Caddy and the Go backend.

### Accepted and rejected values

| Value | Result |
|---|---|
| `https://panel.example.com` | accepted |
| `https://panel.example.com:8443` | accepted (explicit port) |
| `https://panel:8443` | accepted (single-label host, for compose or Kubernetes service names) |
| `https://*.customers.example.com` | accepted (leftmost-label wildcard) |
| `http://localhost:3000` | accepted (loopback only, for development) |
| `https://a.example.com https://b.example.com` | accepted (space separated) |
| `*` or `https://*` | rejected, allowing every origin defeats the allowlist |
| `panel.example.com` | rejected, no scheme |
| `https://panel.example.com/embed` | rejected, no path allowed |
| `https://panel.example.com/` | rejected, drop the trailing slash |
| `https://a.example.com,https://b.example.com` | rejected, commas are not a separator |
| `http://panel.example.com` | rejected, plain HTTP cannot receive the `Secure` session cookie |
| `https://*.com` | rejected, a wildcard needs at least two labels beneath it |
| `'self'` | rejected, CSP keywords are not origins |

Entries are lowercased and deduplicated. A rejection prints the offending token and the reason, and the container exits non-zero rather than starting with a policy you did not intend.

## How framing is allowed

`frame-ancestors` is only honoured on the response that carries the framed **document**. In the container that document is `index.html`, served by Caddy, not by the Go binary. So the value has two emitters:

| Response | Emitted by | Header |
|---|---|---|
| `index.html` and static assets | Caddy (`docker/Caddyfile`) | `Content-Security-Policy: frame-ancestors <list>` |
| `/api/*`, `/healthz`, SSO redirect | Go (`securityHeadersMiddleware`) | full CSP including `frame-ancestors <list>` |

Both read the same environment variable, so the allowlist is defined once and lands on the document and the API responses alike.

The split is safe because `docker/entrypoint.sh` starts Caddy only after the backend answers `/healthz`. Go validates the value first, so a malformed allowlist kills the container before Caddy can serve a single header.

**`X-Frame-Options` is emitted only in the deny case.** It cannot express an allowlist (`ALLOW-FROM` was removed from every shipping engine), and an engine that preferred a stale `X-Frame-Options` over `frame-ancestors` would silently override the allowlist. Restricting it to the deny case means it can only ever agree with `frame-ancestors 'none'`.

## The session cookie

Setting an allowlist switches `gftp_session` from `SameSite=Lax` to `SameSite=None; Secure; Partitioned` **for every user**, framed or not. A cross-site iframe receives no cookie otherwise.

| | Default | With `GFTP_FRAME_ANCESTORS` set |
|---|---|---|
| `SameSite` | `Lax` | `None` |
| `Secure` | when served over TLS | always |
| `Partitioned` | absent | set |
| `HttpOnly`, `Path` | `HttpOnly`, `/` | unchanged |

Three consequences worth knowing before you enable it.

**`Secure` is forced, not derived.** Behind an external TLS terminator, Caddy listens on port 80 inside the container and receives `X-Forwarded-Proto: http`, so deriving `Secure` from the request scheme would produce a cookie the browser silently drops. It is forced instead, and the config rejects non-loopback `http://` ancestors for the same reason.

**A partitioned session is keyed to the top-level site.** Chrome partitions the cookie by the embedding page's scheme plus registrable domain. If the panel and GoblinFTP share a registrable domain (`panel.example.com` framing `files.example.com`), the partition key matches a direct visit and the session is shared. Across different registrable domains it is not: the user is signed in inside the panel and signed out in a top-level tab, which is usually what you want anyway.

**CSRF protection does not weaken.** `SameSite=Lax` was defence in depth alongside the `X-CSRF-Token` header, which remains mandatory on every mutating request. The token is sufficient on its own because GoblinFTP never emits CORS headers: a cross-origin page that sends the header triggers a preflight nothing answers, and one that omits it is rejected. That absence is load-bearing and has a dedicated regression test (`TestNoCORSHeadersEver`). Do not add CORS middleware without revisiting this.

**Known limitation:** the switch is unconditional. Even when every allowlisted origin shares a registrable domain with GoblinFTP (where `SameSite=Lax` would have been sent inside the frame anyway), the cookie still becomes `SameSite=None`. That is a deliberate simplification, not a requirement of the topology.

## Pre-authenticating with SSO

The panel owns the credentials, so it also owns the login. See [SSO login links](../examples/sso/README.md) for the token format and generators in Go, Node.js, and PHP.

Points specific to embedding:

- **Mint a fresh link on every render.** SSO tokens are single-use. A browser reload re-requests the frame's `src`, and a reused token lands on `/login?sso_error=used` with "This SSO login link has already been used" (a 404 when the login form is disabled, see below).
- **Keep the TTL short.** Minutes, not hours. Replay protection is in-memory, so a token becomes replayable if the backend restarts before the token expires.
- **Set `GFTP_LOGIN_FORM_DISABLED=true`.** A signed-out visitor then gets a plain 404 instead of a credential form a panel user cannot fill in, and the preference controls (language, appearance, settings) go with it. Reopening the frame from the panel is the only way back in.
- **Session lifetime is `GFTP_SESSION_TTL_SECONDS` (default 2 hours).** The panel has no way to observe expiry from outside the frame, so a long-lived panel page will eventually land on that 404. Reloading the frame with a fresh link recovers it.
- **Per-tenant themes** ride along on the SSO token via `-tenant <name>`, so one instance can serve a differently branded frame per customer. See [Theming](theming.md).

## Chromeless mode

`GFTP_EMBED_CHROMELESS=auto` (the default) hides duplicate chrome when, and only when, the page is framed. Detection is `window.self !== window.top`, evaluated at boot.

| Hidden when embedded | Kept |
|---|---|
| Logo and app name in the header | Files/Editor navigation (the only route back from the editor) |
| Disconnect button | Theme toggle, keyboard shortcuts, settings |
| Status bar | Breadcrumb, file table, upload panel |
| Attribution footer (login screen, settings dialog) | Everything else |

The header is trimmed, never removed. Disconnect is hidden because a user who disconnects inside a frame has no credentials to reconnect with, and the panel, not the frame, owns the session lifecycle.

Force it with `on` (useful when the panel proxies GoblinFTP at the top level but still wants its own chrome) or disable it with `off`.

**This is presentation only.** It hides UI; it never restricts what a user can do, and nothing on the server branches on it. Anything that must be forbidden inside a panel belongs in server-side configuration: `editor.disabled`, `editor.viewOnly`, `connection.lockHost`, `connection.disableChmod`, `access.allowedClientAddresses`. See [Configuration](configuration.md#settingsjson).

## Browser support

| Browser | Behaviour |
|---|---|
| Chrome, Edge | Works cross-site. `Partitioned` (CHIPS) keeps the cookie available under third-party cookie restrictions. |
| Firefox | Works cross-site. Total Cookie Protection partitions the cookie by top-level site, matching the CHIPS behaviour above. |
| Safari | Blocks third-party cookies by default. A cross-registrable-domain embed does not establish a session, and GoblinFTP does not call the Storage Access API to request an exception. |

Third-party cookie policy changes often and differs between a browser's default and its strict privacy mode, so verify a true cross-site embed in the browsers your customers actually use before rolling it out.

**The recommended topology is a shared parent domain**, for example `panel.example.com` framing `files.example.com`. Same registrable domain means the frame is same-site, which sidesteps third-party cookie policy in every browser including Safari, and makes the partitioned session line up with a top-level visit.

## Verifying the setup

Check the document, not just the API. Verifying only `/api/*` will pass while framing stays wide open, because the document is what the browser enforces against.

```bash
# The SPA document, served by Caddy. This is the one that matters.
curl -sI https://files.example.com/ | grep -i content-security-policy

# The API, served by Go.
curl -sI https://files.example.com/healthz | grep -iE 'content-security-policy|x-frame-options'
```

With no allowlist both show `frame-ancestors 'none'`, and the API response additionally carries `X-Frame-Options: DENY`. With an allowlist both show the exact space-separated list, and `X-Frame-Options` is absent.

The startup log prints the active policy whenever an allowlist is configured:

```
level=INFO msg="iframe embedding enabled" frame_ancestors="https://panel.example.com"
  session_cookie="SameSite=None; Secure; Partitioned" chromeless=auto
```

Then load the panel page and check DevTools. In the Network tab, the SSO redirect response must carry `Set-Cookie: gftp_session=...; SameSite=None; Secure; Partitioned`. In the Application tab, the cookie should be present under the panel's origin.

## Troubleshooting

Four independent failures produce the same symptom, a frame that loads and then redirects to the login screen with nothing in the console. Work through them in this order.

| Symptom | Likely cause | Check |
|---|---|---|
| Frame is blank, console shows a CSP error naming `frame-ancestors` | The panel origin is not allowlisted, or the allowlist is on the API but not the document | `curl -sI https://files.example.com/` and compare the origin exactly, including scheme and port |
| Frame loads, then bounces to the login screen | Session cookie was not stored | DevTools Network tab: does the SSO response `Set-Cookie` carry `SameSite=None; Secure; Partitioned`? |
| Works in Chrome, fails in Safari | Third-party cookie blocking | Move to a shared registrable domain, or accept that Safari is unsupported for cross-site embeds |
| `Set-Cookie` is present but the cookie is not stored | Served over plain HTTP, so `Secure` is rejected | Terminate TLS in front of GoblinFTP and load the frame over `https://` |
| "This SSO login link has already been used" | The token was consumed by an earlier render or a frame reload | Generate the link at render time, never cache the frame URL |
| "Your SSO login link has expired" | Clock skew or a TTL shorter than the page's time to load | Check NTP on both hosts; raise `-ttl` slightly |
| Container refuses to start | An entry in `GFTP_FRAME_ANCESTORS` failed validation | Read the startup error, it names the offending token and the reason |
| Frame works but shows full branding | `GFTP_EMBED_CHROMELESS=off`, or the page is not actually framed | Confirm the value, and that the panel renders a real `<iframe>` |

## Security notes

- **Allowlist exactly the origins you control.** `frame-ancestors` is the only thing standing between your customers' file manager and a clickjacking overlay on someone else's page.
- **Wildcards carry subdomain-takeover risk.** `https://*.example.com` grants framing rights to every subdomain, including one an abandoned DNS record could hand to somebody else. Prefer explicit origins where the set is small.
- **The allowlist is never published to the SPA.** `GET /api/system/vars` is public, and exposing the list would leak your panel domains to any anonymous caller. The browser enforces framing before any JavaScript runs, so the SPA has no use for it.
- **The frame inherits the customer's FTP/SFTP credentials.** Anything the panel user can do in the frame, they can do with a direct client. Embedding changes the interface, not the permissions. Restrictions belong on the FTP/SFTP server.

## See also

- [Configuration](configuration.md) for the full environment-variable reference.
- [SSO login links](../examples/sso/README.md) for token generation.
- [Theming](theming.md) for per-tenant branding inside the frame.
- [Installation](installation.md) for TLS and reverse-proxy setups.

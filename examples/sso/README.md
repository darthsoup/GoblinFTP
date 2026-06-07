# SSO login links

GoblinFTP can log users in via **one-time SSO links** — your application (a hosting
panel, admin backend, …) generates a link containing encrypted FTP/SFTP credentials,
and opening it connects the user without showing the login form.

## Setup

Run GoblinFTP with SSO enabled:

```bash
GFTP_SSO_ENABLED=true
GFTP_SSO_SECRET=<long random string>   # shared with the link generator — keep it secret
```

## How it works

```
your app                          GoblinFTP backend                     browser/SPA
   │  encrypt credentials with        │                                     │
   │  GFTP_SSO_SECRET → token         │                                     │
   │──── redirect user to /?sso=<token> ──►                                 │
   │                                  │ decrypt + validate token,           │
   │                                  │ mark it used (replay protection),   │
   │                                  │ create session, redirect to /login  │
   │                                  │────────────────────────────────────►│
   │                                  │  GET /api/auth/status               │
   │                                  │◄── { ssoAutoConnect: true, csrfToken }
   │                                  │  POST /api/auth/sso-connect         │
   │                                  │◄── dials FTP/SFTP → connected       │
```

The link is **single-use** (replayed tokens are rejected) and **expires** at the
`exp` timestamp baked into the token — keep the TTL short (minutes, not hours).

## Generating links

### Go CLI (in this repo)

Reuses the server's own `internal/sso` package, so it is always format-compatible:

```bash
just sso-link -host ftp.example.com -username alice -password s3cret \
  -base-url https://files.example.com
# or directly:
cd backend && GFTP_SSO_SECRET=change-me go run ./cmd/gftp-sso-link \
  -host ftp.example.com -username alice -password s3cret
```

Flags: `-protocol ftp|sftp` (default `ftp`), `-port` (defaults to 21/22), `-ttl`
(default `5m`), `-dir`, `-lang`, `-base-url`, `-secret` (or `$GFTP_SSO_SECRET`).
The password can also come from `$GFTP_SSO_PASSWORD` to keep it out of shell history.

### Node.js — [`node/generate-sso-link.mjs`](node/generate-sso-link.mjs)

Stdlib only (Node ≥ 18):

```bash
GFTP_SSO_SECRET=change-me node node/generate-sso-link.mjs \
  --host ftp.example.com --username alice --password s3cret \
  --base-url https://files.example.com
```

### PHP — [`php/generate_sso_link.php`](php/generate_sso_link.php)

Works as a CLI script or as a drop-in function for your application:

```php
require 'generate_sso_link.php';

$url = gftp_sso_link($secret, 'https://files.example.com', [
    'protocol' => 'sftp',
    'host'     => 'sftp.example.com',
    'username' => 'alice',
    'password' => 's3cret',
]);
header('Location: ' . $url);
```

## Token format

For implementing a generator in any other language
(reference: [`backend/internal/sso/token.go`](../../backend/internal/sso/token.go)):

1. **Payload** — JSON object:

   | Field | Type | Notes |
   |---|---|---|
   | `type` | string | `"ftp"` or `"sftp"` |
   | `host` | string | server hostname |
   | `port` | int | server port |
   | `username` | string | login user |
   | `password` | string | login password |
   | `initialDirectory` | string | optional hint (currently the server's working directory wins) |
   | `language` | string | optional UI language hint (`en`, `de`) |
   | `exp` | int | Unix timestamp; token rejected after this |

2. **Key derivation** — `HKDF-SHA256(secret, salt = empty, info = "gftp-sso")`, 32 bytes.
3. **Encryption** — AES-256-GCM with a random 12-byte IV; 16-byte auth tag.
4. **Wire format** — `iv || tag || ciphertext`, encoded as **base64url without padding**
   (RFC 4648 §5).
5. **Link** — `https://<goblinftp>/?sso=<token>`.

## Security notes

- Links contain (encrypted) credentials — generate them on demand, deliver over
  HTTPS only, and never log or persist them.
- Use a long random `GFTP_SSO_SECRET` (32+ bytes) and treat it like a password.
- Replay protection is in-memory: a token becomes reusable again if the backend
  restarts before it expires — another reason to keep TTLs short.

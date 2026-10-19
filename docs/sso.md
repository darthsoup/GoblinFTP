---
description: Let users sign in from your app with a one-time login link.
---

# SSO login links

Let users open GoblinFTP from your app without typing their FTP password again. Your backend creates an encrypted link that signs them in. Each link works once and expires after a few minutes.

## Setup

Enable SSO on the GoblinFTP server:

```bash
GFTP_SSO_ENABLED=true
GFTP_SSO_SECRET=replace-with-your-shared-secret
```

Use a random secret of at least 32 bytes. You can generate one with `openssl rand -hex 32`. Your app and GoblinFTP must use the same secret. Keep it in backend code or server settings, never in browser code.

## How it works

1. Your backend creates a link using the user's server address and login details.
2. The user opens the link, or your app loads it inside an iframe.
3. GoblinFTP checks the link, connects to the server, and opens the file manager.

Create a fresh link each time a user opens GoblinFTP. The default expiry is five minutes.

## Generating links

Run these commands from the repository root. All three generators read `GFTP_SSO_SECRET` and `GFTP_SSO_PASSWORD` from your environment, so you do not need to put them in the command.

### Go CLI

```bash
just sso-link -host ftp.example.com -username alice \
  -base-url https://files.example.com
```

The Go generator supports FTP, FTPS, and SFTP. Use `-protocol sftp` or `-protocol ftps` to change the default FTP connection.

### Node.js

The [Node.js generator](https://github.com/darthsoup/goblinftp/blob/main/examples/sso/node/generate-sso-link.mjs) needs Node 18 or newer. No extra packages are required.

```bash
node examples/sso/node/generate-sso-link.mjs \
  --host ftp.example.com --username alice \
  --base-url https://files.example.com
```

This example supports FTP and SFTP. It does not accept FTPS.

### PHP

The [PHP generator](https://github.com/darthsoup/goblinftp/blob/main/examples/sso/php/generate_sso_link.php) needs PHP 7.1.2 or newer with OpenSSL.

```bash
php examples/sso/php/generate_sso_link.php \
  --host=ftp.example.com --username=alice \
  --base-url=https://files.example.com
```

Use `--name=value` for optional PHP arguments. The script passes the protocol to GoblinFTP, which accepts `ftp`, `ftps`, and `sftp`.

You can also use the PHP function in your app:

```php
require __DIR__ . '/examples/sso/php/generate_sso_link.php';

$url = gftp_sso_link($secret, 'https://files.example.com', [
    'protocol' => 'sftp',
    'host' => 'sftp.example.com',
    'username' => 'alice',
    'password' => $password,
]);
header('Location: ' . $url);
```

Adjust the include path for your project. The function also accepts `port`, `initialDirectory`, `language`, `tenant`, and `ttlSeconds` in the connection array.

### Options

The host and username are required. Other options are optional:

| Setting | Go | Node.js and PHP | Default |
|---|---|---|---|
| Server address | `-host` | `--host` | Required |
| Username | `-username` | `--username` | Required |
| Connection type | `-protocol` | `--protocol` | `ftp` |
| Server port | `-port` | `--port` | 22 for SFTP, 21 otherwise |
| Password | `-password` | `--password` | `GFTP_SSO_PASSWORD` |
| Starting folder hint | `-dir` | `--dir` | Empty |
| Language | `-lang` | `--lang` | Empty |
| Customer theme | `-tenant` | `--tenant` | Empty |
| Link expiry | `-ttl` | `--ttl-seconds` | `5m` for Go, `300` seconds otherwise |
| GoblinFTP URL | `-base-url` | `--base-url` | `http://localhost:8080` |
| Shared secret | `-secret` | `--secret` | `GFTP_SSO_SECRET` |

## Token format

You only need this section if you are writing your own link generator. See the [server implementation](https://github.com/darthsoup/goblinftp/blob/main/backend/internal/sso/token.go) for a reference.

::: details Token format reference

The encrypted payload is a JSON object:

| Field | Type | Meaning |
|---|---|---|
| `type` | string | `ftp`, `ftps`, or `sftp` |
| `host` | string | Server hostname |
| `port` | int | Server port |
| `username` | string | Login user |
| `password` | string | Login password |
| `initialDirectory` | string | Optional folder hint; the server's working directory currently wins |
| `language` | string | Optional language, such as `en` or `de` |
| `tenant` | string | Optional [theme name](theming.md) |
| `exp` | int | Expiry time as a Unix timestamp |

1. Derive a 32-byte key using `HKDF-SHA256(secret, salt = empty, info = "gftp-sso")`.
2. Encrypt the JSON with AES-256-GCM, a random 12-byte IV, and a 16-byte authentication tag.
3. Join `iv || tag || ciphertext` and encode it as base64url without padding (RFC 4648 section 5).
4. Use the result in `https://files.example.com/?sso=<token>`.

:::

## Security notes

- Use HTTPS and create links only when needed. Never log or save them: they contain encrypted passwords.
- Keep the shared secret private.
- Keep link expiry short. Restarting the backend clears the list of used links, so an unexpired link could work again after a restart.

## See also

- [Iframe embedding](embedding.md) to show GoblinFTP inside your app.
- [Branding and themes](theming.md) to choose a customer theme in the link.
- [Configuration](configuration.md#sso-login-links) for SSO settings.

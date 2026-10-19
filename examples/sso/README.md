# SSO login link generators

See the [SSO login links guide](../../docs/sso.md) for setup, generator options, token format, and security notes. The guide is also available in the [published documentation](https://darthsoup.github.io/GoblinFTP/sso.html).

- [Go CLI](../../backend/cmd/gftp-sso-link/main.go): run `just sso-link` from the repository root.
- [Node.js generator](node/generate-sso-link.mjs): standard library only, Node 18 or newer.
- [PHP generator](php/generate_sso_link.php): CLI script or backend function, PHP 7.1.2 or newer with OpenSSL.

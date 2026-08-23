# Security Policy

GoblinFTP handles FTP/SFTP credentials and proxies file content, so security
reports are taken seriously and get priority over feature work.

## Supported versions

Only the latest released version receives security fixes. Fixes are published as
a new patch release and a new `ghcr.io/darthsoup/goblinftp` image.

## Reporting a vulnerability

Please report privately, not as a public issue.

- Preferred: open a [security advisory](https://github.com/darthsoup/goblinftp/security/advisories/new)
  on the repository.

Helpful details: affected version (the settings modal footer and `/healthz`
report it), deployment shape (single container, behind a reverse proxy,
embedded in an iframe, SSO), reproduction steps, and impact.

Expect an acknowledgement within a few days. Please give a reasonable window for
a fix before public disclosure.

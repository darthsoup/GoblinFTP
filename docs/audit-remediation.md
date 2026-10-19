# Audit remediation and upgrade notes

This change incorporates the first-wave fixes from PR #44 and addresses the second audit dated 2026-10-19. The application remains stateless. Sessions, admission counters, pending SSO credentials, and storage reservations are held in memory.

## Deployment changes

- The standalone backend now binds to `127.0.0.1`. Set `GFTP_BIND_ADDRESS=0.0.0.0` explicitly when another container or machine must reach it, and restrict that cleartext listener to your TLS proxy.
- Metrics also bind to loopback. Containerized Prometheus installations must explicitly set `GFTP_METRICS_BIND_ADDRESS=0.0.0.0` and limit access through their network policy. Metrics remain opt-in and separate from the application listener.
- Unknown `GFTP_*` names fail startup. Renamed variables retain their migration diagnostics. `GFTP_VERSION`, `GFTP_DEV_PROXY`, `GFTP_SSO_PASSWORD`, documented integration-test variables, and the two internal Caddy variables are exceptions.
- Configured SSO and download-token secrets must contain at least 32 bytes. Generate independent random values, for example with `openssl rand -hex 32`. An unset download secret is still generated automatically. A configured SSO secret must match every link generator.
- HTTPS uses `__Host-gftp_session`. Existing HTTPS sessions require a fresh login. Legacy cookies are deleted and ignored on HTTPS. Plain HTTP development uses the unprefixed cookie. Duplicate session cookies are rejected; a fresh login replaces the browser cookie.
- Preset host addresses are hidden from anonymous system variables unless `GFTP_CONNECTION_EXPOSE_PRESET=true`. A locked login form displays a fixed-server placeholder and the backend supplies its configured target. Private-network destinations remain supported.
- Caddy's container listener uses HTTP/1.1. An external TLS terminator can still negotiate HTTP/2 with browsers. The Caddy executable is rebuilt from its own locked module with patched Go and `x/net` versions.

See the generated resource-protection table in [Configuration](configuration.md) for all limits. Capacity values must be positive. Large installations and shared NAT networks may need larger per-IP session or login budgets.

## SSO confirmation

Opening a one-time SSO link creates a pending login, without connecting or replacing an active session. The confirmation page shows the protocol, target, and account. The user must choose Connect; Cancel preserves any existing connection.

Pending credentials use a separate host-only cookie and CSRF token. They expire after five minutes or the token's expiry, whichever comes first. Finalization rotates the active session and CSRF token. Unknown SFTP host keys still require confirmation. Link reuse is rejected, including alternate Base64 encodings. One-time consumption remains in memory and is lost on restart, as before.

SSO `initialDirectory` is checked against the remote server and used as the starting directory, not a confinement boundary. Supported SSO languages override the administrator default; an explicit browser language preference wins.

Integrations using `/api/auth/sso-connect` must first read `/api/auth/sso-pending` with the pending cookie and use its CSRF token. The previous automatic connection behavior has been removed.

## Connection loss and file safety

Open destructive dialogs and clipboard operations are cancelled on connection loss. In-flight requests and previews are aborted. Editor buffers are preserved as detached drafts, labelled with their original server. Export them locally, open the destination file afresh, and transfer the desired content manually. Detached drafts cannot save, reload, or autosave to a new connection. Draft contents are not written to browser storage.

Delete validation rejects a whole batch before any operation when one target is invalid. Remote permission failures still use per-item results. FTP checks both separator interpretations, Windows dot/space root aliases, and drive/UNC forms. SFTP permits literal POSIX backslashes. FTP directory deletion uses bounded traversal and never falls back to recursively deleting an unclassified target.

Copy and rename reject overlapping sources and destinations. Upload commit renaming must remain in the reserved parent. Existence checks fail closed on ambiguous or incomplete responses. FTP fallback collision checks are case-insensitive. These checks do not provide atomic non-overwrite guarantees against independent writers on the remote server.

## Resource limits

- Sessions: 100 globally and 10 per client IP, including pending logins.
- In-flight connects: 32 globally and 4 per IP. Every admitted dial, including host-key probes and successful logins, consumes IP admission budget. A successful login does not reset other requests' failures.
- Temporary storage: 4 GiB globally and 1 GiB per session; 16 pending uploads per session. At most 100,000 chunk slots may be reserved across the instance, including uploads with tiny chunk sizes. Multipart request reservations and retained chunks can overlap during a chunk upload, so leave room above the intended upload size.
- Listings: 100,000 entries and 16 MiB of listing data. A limit error returns no partial listing.
- Remote recursive walks: 64 levels and 100,000 entries per operation.
- Archives: 512 MiB payload/expansion bounds, 10,000 extracted entries, and 4 MiB ZIP directory metadata. The ZIP preflight accepts ordinary single-disk ZIP/ZIP64 layouts; ambiguous offsets, prepended executable layouts, and multi-disk archives are rejected before allocation.
- Archive creation and remote-copy spools also enforce actual written-byte limits of 512 MiB. Remote size declarations cannot bypass those limits. Four listing/archive/copy operations may run concurrently across the instance.
- Previews and thumbnails stop after 5 MiB and 3 MiB of received bytes respectively, including when a remote server understates the file size.

Existing local temporary files are charged at startup until removal succeeds. S3 inventory is streamed and reconciled periodically, excluding uploads already covered by live reservations. Inventory failures reject storage admission rather than assume the bucket is empty. Storage shared with another running GoblinFTP instance needs an external quota; in-memory reservations are per process.

## Dependency maintenance

Container base images use immutable multi-platform digests. GitHub Actions use full commit SHAs with the originating tag in a comment. Resolve new tags to their exact revisions when updating, review the dependency diff, and rerun the shared checks.

The FTP and SFTP modules are replaced by immutable pseudo-versions from [darthsoup/ftp](https://github.com/darthsoup/ftp) and [darthsoup/sftp](https://github.com/darthsoup/sftp), pinned in `backend/go.mod` and verified by `backend/go.sum`. Dependency source is not copied into this repository. Both adapters explicitly request bounded listings; the original upstream listing APIs retain their existing behavior.

The patches are proposed upstream in [jlaffaye/ftp#478](https://github.com/jlaffaye/ftp/pull/478) and [pkg/sftp#698](https://github.com/pkg/sftp/pull/698). FTP additionally clears completion deadlines and closes connections with unrecoverable completion replies. Until upstream releases these changes, review upstream updates and refresh the fork commits deliberately. Remove each replacement only when its upstream release provides equivalent protections and the adapter tests pass. The fork repositories retain the full upstream test suites and licenses.

The dedicated Caddy module is in `docker/caddy`. Keep its lockfile and Go/toolchain security floor updated independently from the backend. Container CI scans the actual shipped Go executables, not just the backend source module. Both binaries retain their symbol tables so the scan can distinguish linked code from unused packages in a dependency module.

## Finding-to-validation checklist

| Findings | Validation |
|---|---|
| PR #44 FTPS, download order, canonical SSO, archive containment, SFTP setup, SVG, symlinks, paste | Adapter, download-order, SSO canonicalization/replay, archive path, SFTP setup/delete, and frontend file-store tests |
| R2-01, R2-07, R2-12 | Concurrent account reservations, retained failure/IP charges, session reservation limits, blocked-dial API test |
| R2-02, R2-15 | ZIP preflight entry/count tests; bounded FTP LIST/MLSD and SFTP READDIR tests |
| R2-03, R2-08 | FTP traversal and deadline tests; FTPS data-channel integration fixture |
| R2-04, R2-16 | Detached draft/save guards, connection-epoch cancellation, filename escaping, modal reset tests |
| R2-05, R2-06, R2-22 | Actual Caddy build metadata and vulnerability scan, HTTP/1-only handshake, container headers/runtime smoke tests, config defaults |
| R2-09, R2-10 | Legacy backslash delete and trailing-dot/space prevalidation tests. Native Windows FTP remains an external compatibility check. |
| R2-11, R2-13, R2-18 | Pending SSO confirmation, preservation of active sessions, cookie duplication/prefix tests, initial-directory/language behavior |
| R2-14, R2-19, L-03 | Unknown-variable, secret-length, safe-default, and preset-visibility tests |
| R2-17, R2-20, R2-21 | Colon archive names, SFTP deletion, backend copy/move overlap tests |
| H-02, H-03, H-06 | Upload parent confinement, existence/collision tests, unsafe archive output child-name tests |
| H-05, M-06, M-08 | Storage reservation, exact chunk/commit byte validation, sequential chunk assembly tests |
| M-05, M-09 | Private/no-store response headers and known-host trust/re-pin tests |

A passing mock test is not a native Windows FTP reproduction. No native Windows FTP server is included in this workspace.

## Recorded validation (2026-10-19)

The remediation branch passed these checks against the local source and `goblinftp:audit-remediation` image:

- Backend `go test -race ./...`, including the API listener tests, and separate race runs in both patched protocol modules. The final control-character validation addition also passed the transfer package race tests.
- `golangci-lint run`: zero issues. Frontend ESLint and Nuxt TypeScript checks passed.
- Frontend Vitest: 142 tests across 18 files passed. Locale parity passed for all 13 languages and all 36 backend error codes. Generated configuration parity passed with the backend tests.
- Docker build, including SPA generation, and seven container smoke tests passed on the local Linux arm64 image. These cover startup/shutdown, proxy identity, persistent-volume permissions, invalid configuration, an unavailable S3 endpoint, HTTP/1-only behavior, and conditional framing headers.
- The shipped Caddy configuration validated. Build metadata identifies Caddy v2.11.7, Go 1.26.9, and `golang.org/x/net` v0.61.0.
- `govulncheck` v1.9.0 scanned both actual shipped executables and reported zero reachable vulnerabilities and zero vulnerable imported packages. Each scan also reported one advisory in an unused required module. This is a symbol-aware executable scan, not an operating-system package scan.
- Documentation generation, workflow actionlint, and `git diff --check` passed.

Native Windows FTP behavior was unavailable in this validation run; an amd64 container runtime was not exercised. R2-10 retains that compatibility qualification. The protocol fixtures validate FTPS data encryption and TLS session reuse locally; they do not establish compatibility with every FTP server product.

## Dependency fork validation

The copied dependency modules were replaced with pinned forks after opening the upstream pull requests. FTP's complete race suite in `-short` mode and `go vet ./...` passed. Short mode skips its pre-existing fixed-port test because a local server occupies port 2121. SFTP's full race suite and its `-integration -testserver` race run passed using an isolated temporary directory. SFTP's standalone vet diagnostics match the unchanged upstream baseline.

After the fork migration, GoblinFTP's complete backend race suite, backend lint, module checksum verification, documentation build, and workflow actionlint passed. The rebuilt Linux arm64 image passed all seven smoke tests. Both shipped executables again scanned with zero reachable vulnerabilities and zero vulnerable imported packages; the unused-module advisory qualification above still applies.

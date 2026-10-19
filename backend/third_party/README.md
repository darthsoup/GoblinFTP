# Local protocol extensions

The pinned FTP and SFTP libraries buffer complete directory listings. Their public APIs do not provide all the cancellation and entry-limit guarantees needed by directory-size scans.

The module replacements in backend/go.mod use local copies of their production sources and licenses. Scan extensions are isolated in ftp/directory_context.go and sftp/scan.go. Provenance and maintenance notes live in each directory's UPSTREAM.md.

Update the local source copies together with their pinned module versions. Validate bounded listings, cancellation during control and data I/O, cleanup, subsequent connection use, and normal transfers against the protocol test servers. The Docker build copies these modules before resolving Go dependencies.

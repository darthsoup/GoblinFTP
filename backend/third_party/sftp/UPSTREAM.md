# SFTP source

Based on github.com/pkg/sftp v1.13.11. The upstream license and contributor list are retained in LICENSE and CONTRIBUTORS.

Local extensions in scan.go add LstatContext and ReadDirLimitContext. Directory reads retain at most the requested entry limit, stop after one lookahead entry, and use the operation context for handle cleanup. Upstream transfer methods keep their existing behavior.

The application adapter closes the underlying SSH transport if a scan is cancelled during I/O, including a blocked write or cleanup. It waits for that cancellation callback before releasing the session transfer lock and reports connection loss to the API.

When updating the pinned dependency, copy its production sources and internal packages, retain the license, and reapply scan.go. Run the application SFTP protocol, integration, and race tests. Upstream comment punctuation is normalized to the repository style.

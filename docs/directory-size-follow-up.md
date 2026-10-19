# Directory size follow-up plan

This checklist records the completed changes from the directory-size implementation review. Protocol extensions and their maintenance notes are documented in [backend/third_party](../backend/third_party/README.md).

## 1. Bound cancellation and metadata operations (P2)

- [x] Make scan metadata requests obey cancellation and the scan deadline, including SFTP directory-handle cleanup and the initial metadata request.
- [x] Define how each adapter safely stops an in-flight operation. If the connection cannot remain usable, close it and surface the existing connection-loss state.
- [x] Hold the transfer lock until the underlying operation has actually stopped. Do not return early by abandoning a goroutine that still owns the shared connection.
- [x] Release the per-session scan guard after cleanup on success, cancellation, timeout, and failure.

The review reproduced an SFTP `Lstat` that exceeded its scan budget and a cancelled `ReadDirContext` that remained blocked waiting for `CLOSE`. Both operations finished only after the test connection was closed.

Acceptance: a silent test server cannot retain the transfer lock indefinitely. After cancellation, either another file operation succeeds or the session reports a connection loss and allows reconnection. Check cancellation while waiting for the lock, during metadata reads, during directory reads, and during cleanup.

## 2. Enforce the entry budget while reading (P2)

- [x] Check the remaining budget before descending into another directory.
- [x] Add bounded or incremental directory reads at the adapter/library boundary. Enforce the remaining entry budget as entries arrive.
- [x] Mark a truncated result as partial and preserve the accumulated lower bound.
- [x] Verify the FTP and SFTP library capabilities before choosing the implementation. Wrapping a fully buffered `List` call does not bound its memory use or remote work.

Before this change, the walker checked its entry limit after receiving the complete listing. A review reproduction with a budget of two entries still materialized 100,000 records.

Acceptance: a large flat directory cannot force allocation of its entire listing. Reaching the limit prevents additional traversal, and the adapter safely completes or stops the current protocol operation.

## 3. Distinguish physical emptiness from excluded contents (P3)

- [x] Track whether the root directory contains entries independently of the file and folder totals used for size calculation.
- [x] Keep symbolic-link targets excluded from recursive totals.
- [x] Show `Empty` only when the directory is known to contain no entries. Leave a complete zero-byte size blank when contents exist.
- [x] Keep partial zero-byte results labelled as partial.

Before this change, excluded symbolic links left both counts at zero and incorrectly satisfied the frontend's `Empty` condition. The result now includes `hasEntries` independently of those counts.

Acceptance: cover an empty directory, only symbolic links, zero-byte files, empty child directories, hidden entries, and an incomplete zero-byte result. Check the shared label in rows, cards, and Properties.

## 4. Keep the last result until explicit invalidation

- [x] Remove automatic expiry of displayed totals. Keep results until refresh, a file change, or disconnect.
- [x] Remove the unused production cache-reuse branch and its `force` argument. The Properties action always requests a fresh calculation.
- [x] Separate the last successful result from loading and error state.
- [x] Preserve the previous total while recalculating, after cancellation while the connection remains active, and after a failed recalculation. Continue offering cancel or retry as appropriate.
- [x] Retain abort and generation guards so cancelled or superseded requests cannot overwrite current state.

Acceptance: totals and counts remain visible after 60 seconds, and expiry no longer reorders size-sorted directories. Cancelling or failing a recalculation preserves the previous result while the connection remains active; explicit invalidation or connection loss clears it.

## 5. Use one small scan interface

- [x] Give the traversal engine a dedicated context-aware metadata interface with bounded directory reads and explicit truncation.
- [x] Replace the combination of synchronous methods and optional context interfaces in the scan path.
- [x] Make the session wrapper implement only the scan operations it needs, without embedding the full transfer client.
- [x] Keep session liveness checks, transfer locking, and safe API error mapping at the existing boundaries.

Acceptance: both protocol adapters satisfy the same scan contract. Tests can express cancellation and entry limits directly, without relying on a fallback to a context-free operation.

## 6. Centralize file changes and size invalidation

- [x] Move directory-size endpoint knowledge out of the generic API helper into a shared file-operation layer.
- [x] Cover rename, copy, delete, directory and file creation, writes, uploads and upload commits, extraction, compression, and permission changes where relevant.
- [x] Retain protection against scans racing with mutations. Invalidate at the required operation boundaries or use a mutation revision checked before accepting scan results.
- [x] Scope invalidation to the connection generation so a late response from an old connection cannot clear results in a new session.
- [x] Keep CSRF handling, envelope parsing, error normalization, and abort support in `useApi`.

Acceptance: reads and upload staging preserve totals, mutations reject stale scan responses, and reconnecting isolates the new session from old requests. Components do not each need their own invalidation logic.

## Delivery and verification

Cancellation, bounded reads, the scan interface, emptiness detection, persistent results, and mutation ownership are implemented. The development documentation describes the resulting behavior. Validation includes the full Go suite, 180 frontend tests, lint, typecheck, locale parity, repeated cancellation race regressions, and real FTP, FTPS, and SFTP test containers.

- [x] Add permanent regressions for the review reproductions, including a silent SFTP server during metadata and cleanup operations.
- [x] Run the transfer, API, auth, FTP, and SFTP suites, plus race checks for scan cancellation and session locking.
- [x] Run frontend store, API, component, sorting, and settings migration tests; then lint, typecheck, and locale parity checks.
- [x] Reuse running FTP and SFTP test containers and preserve their volumes. Start missing containers with `just ftp-up`, `just ftps-up`, or `just sftp-up`. Fixtures must use unique paths and clean up only their own data.

```bash
just test
just lint
just i18n-check
GFTP_TEST_FTP_HOST=localhost:21 GFTP_TEST_FTPS_HOST=localhost:2121 GFTP_TEST_SFTP_HOST=localhost:2222 go -C backend test -race ./internal/ftp/... ./internal/sftp/...
```

Verify in the app through Properties, then check the same result in list and card views. Confirm that calculation remains absent from dropdown menus and that timestamps and cache details remain hidden.

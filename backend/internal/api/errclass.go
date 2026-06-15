// backend/internal/api/errclass.go
package api

import (
	"strings"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
)

// classify maps a raw transfer.Client error to a stable API error code and a
// short, human-friendly message. The raw error is NEVER part of the returned
// message — callers attach it via WithCause for server-side logs only, so raw
// protocol strings (e.g. `550 "Remove directory operation failed."`) never reach
// the client.
//
// Matching is case-insensitive substring on err.Error(), which covers both FTP
// (`550 ...`) and SFTP (`sftp: "..." (SSH_FX_...)`) phrasing. Server wording
// varies, so rules run most-specific first and ErrOperationFailed is the
// catch-all. Patterns are exercised by errclass_test.go.
func classify(err error) (gftperrors.Code, string) {
	if err == nil {
		return gftperrors.ErrOperationFailed, "The operation could not be completed."
	}
	if isConnLost(err) {
		return gftperrors.ErrConnectionLost, "The connection to the server was lost."
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not empty") || strings.Contains(msg, "remove directory operation failed"):
		return gftperrors.ErrDirNotEmpty, "The folder is not empty or is in use."
	case strings.Contains(msg, "permission denied") || strings.Contains(msg, "access is denied") ||
		strings.Contains(msg, "access denied") || strings.Contains(msg, "553"):
		return gftperrors.ErrFilePermission, "Permission denied by the server."
	case strings.Contains(msg, "no such file") || strings.Contains(msg, "no such directory") ||
		strings.Contains(msg, "does not exist") || strings.Contains(msg, "not found"):
		return gftperrors.ErrFileNotFound, "The item no longer exists on the server."
	case strings.Contains(msg, "quota") || strings.Contains(msg, "no space") ||
		strings.Contains(msg, "disk full") || strings.Contains(msg, "552"):
		return gftperrors.ErrQuotaExceeded, "The server is out of storage space."
	default:
		return gftperrors.ErrOperationFailed, "The operation could not be completed."
	}
}

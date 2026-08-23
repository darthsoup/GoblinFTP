package api

import (
	"strings"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
)

// classify maps a raw transfer.Client error to a stable API code and a friendly
// message. The raw text NEVER enters it: callers pass it to WithCause for logs.
func classify(err error) (gftperrors.Code, string) {
	if err == nil {
		return gftperrors.ErrOperationFailed, "The operation could not be completed."
	}
	if isConnLost(err) {
		return gftperrors.ErrConnectionLost, "The connection to the server was lost."
	}
	msg := strings.ToLower(err.Error())
	// Case-insensitive substring, since server wording varies: most-specific
	// rules first, ErrOperationFailed the catch-all (see errclass_test.go).
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

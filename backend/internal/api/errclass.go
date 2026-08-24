package api

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// friendlyMessage is the single source of user-facing text per code, so a code
// substituted by failClient can never contradict the message beside it.
var friendlyMessage = map[gftperrors.Code]string{
	gftperrors.ErrConnectionLost:          "The connection to the server was lost.",
	gftperrors.ErrConnectionTimeout:       "The server stopped responding.",
	gftperrors.ErrDataConnectionFailed:    "The data connection to the server failed. Passive mode, NAT, or a firewall may be blocking it.",
	gftperrors.ErrTransferIncomplete:      "The transfer ended early, so the file may be incomplete on the server.",
	gftperrors.ErrDirNotEmpty:             "The folder is not empty or is in use.",
	gftperrors.ErrFilePermission:          "Permission denied by the server.",
	gftperrors.ErrFileNotFound:            "The item no longer exists on the server.",
	gftperrors.ErrFileExists:              "An item with that name already exists on the server.",
	gftperrors.ErrQuotaExceeded:           "The server is out of storage space.",
	gftperrors.ErrPermissionsNotSupported: "This server does not support changing permissions.",
	gftperrors.ErrInvalidType:             "That operation is not valid for this item.",
	gftperrors.ErrAuthFailed:              "The server rejected the credentials.",
	gftperrors.ErrConnectionFailed:        "The server could not be reached.",
	gftperrors.ErrListFailed:              "The folder could not be listed.",
	gftperrors.ErrStorageUnavailable:      "Temporary storage is unavailable.",
	gftperrors.ErrOperationFailed:         "The operation could not be completed.",
}

// messageFor returns the canonical text for a code, falling back to the
// catch-all so a caller can never emit an empty message.
func messageFor(code gftperrors.Code) string {
	if msg, ok := friendlyMessage[code]; ok {
		return msg
	}
	return friendlyMessage[gftperrors.ErrOperationFailed]
}

// classify maps a raw transfer.Client error to a stable API code and a friendly
// message. The raw text NEVER enters it: callers pass it to WithCause for logs.
//
// Structural sentinels win over text. Substring rules are the last resort, for
// servers whose only signal is the wording of a 550 or an SSH_FX_FAILURE.
func classify(err error) (gftperrors.Code, string) {
	code := classifyCode(err)
	return code, messageFor(code)
}

func classifyCode(err error) gftperrors.Code {
	if err == nil {
		return gftperrors.ErrOperationFailed
	}

	switch {
	case isConnLost(err):
		return gftperrors.ErrConnectionLost
	case errors.Is(err, transfer.ErrConnectionTimeout), isTimeoutErr(err):
		return gftperrors.ErrConnectionTimeout
	case errors.Is(err, transfer.ErrDataConnectionFailed):
		return gftperrors.ErrDataConnectionFailed
	case errors.Is(err, transfer.ErrTransferIncomplete):
		return gftperrors.ErrTransferIncomplete
	case errors.Is(err, transfer.ErrDirNotEmpty):
		return gftperrors.ErrDirNotEmpty
	case errors.Is(err, transfer.ErrPermissionDenied):
		return gftperrors.ErrFilePermission
	case errors.Is(err, transfer.ErrNotFound):
		return gftperrors.ErrFileNotFound
	case errors.Is(err, transfer.ErrFileExists):
		return gftperrors.ErrFileExists
	case errors.Is(err, transfer.ErrQuotaExceeded):
		return gftperrors.ErrQuotaExceeded
	case errors.Is(err, transfer.ErrPermissionsNotSupported):
		return gftperrors.ErrPermissionsNotSupported
	case errors.Is(err, transfer.ErrInvalidType):
		return gftperrors.ErrInvalidType
	case errors.Is(err, transfer.ErrAuthFailed):
		return gftperrors.ErrAuthFailed
	case errors.Is(err, transfer.ErrConnectionFailed):
		return gftperrors.ErrConnectionFailed
	}

	return classifyText(err)
}

// classifyText is the fallback for servers whose only distinguishing signal is
// free-form wording. It reads the deepest message so an outer wrapper carrying a
// remote path cannot steer the result.
func classifyText(err error) gftperrors.Code {
	msg := strings.ToLower(deepestMessage(err))
	switch {
	case contains(msg, "not empty", "remove directory operation failed"):
		return gftperrors.ErrDirNotEmpty
	case contains(msg, "permission denied", "access is denied", "access denied"):
		return gftperrors.ErrFilePermission
	case contains(msg, "already exists", "file exists", "destination exists"):
		return gftperrors.ErrFileExists
	case contains(msg, "no such file", "no such directory", "does not exist", "not found"):
		return gftperrors.ErrFileNotFound
	case contains(msg, "quota", "no space", "disk full", "storage allocation"):
		return gftperrors.ErrQuotaExceeded
	default:
		return gftperrors.ErrOperationFailed
	}
}

// deepestMessage unwraps to the innermost error. The adapters wrap with sentinels
// whose own text ("not found") would otherwise re-match the substring rules.
func deepestMessage(err error) string {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err.Error()
		}
		err = unwrapped
	}
}

func contains(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// isTimeoutErr excludes context.Canceled deliberately: a user-canceled request
// is not a server that stopped responding.
func isTimeoutErr(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

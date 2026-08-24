package sftp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"

	"github.com/pkg/sftp"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// wrapErr maps a pkg/sftp error onto a transfer sentinel so handlers classify
// structurally. errors.Is against the exported fxerr values never matches a
// *StatusError (different types, neither implements Is), so FxCode is the only
// reliable comparison.
func wrapErr(err error) error {
	if err == nil {
		return nil
	}

	var se *sftp.StatusError
	if errors.As(err, &se) {
		switch se.FxCode() {
		case sftp.ErrSSHFxNoSuchFile:
			return fmt.Errorf("%w: %w", transfer.ErrNotFound, err)
		case sftp.ErrSSHFxPermissionDenied:
			return fmt.Errorf("%w: %w", transfer.ErrPermissionDenied, err)
		case sftp.ErrSSHFxOpUnsupported:
			return fmt.Errorf("%w: %w", transfer.ErrPermissionsNotSupported, err)
		case sftp.ErrSSHFxNoConnection, sftp.ErrSSHFxConnectionLost:
			return fmt.Errorf("%w: %w", transfer.ErrConnectionLost, err)
		case sftp.ErrSSHFxBadMessage:
			return fmt.Errorf("%w: %w", transfer.ErrInvalidType, err)
		}
		// SSH_FX_FAILURE is the catch-all servers use for quota, non-empty
		// directory, and existing destination, so its text is all there is.
		return err
	}

	if isTimeout(err) {
		return fmt.Errorf("%w: %w", transfer.ErrConnectionTimeout, err)
	}
	if isSocketLoss(err) {
		return fmt.Errorf("%w: %w", transfer.ErrConnectionLost, err)
	}
	return err
}

func isTimeout(err error) bool {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// isSocketLoss reports a dead transport. os.ErrClosed is excluded: pkg/sftp
// returns it for use-after-Close, which is a caller bug, not a lost connection.
func isSocketLoss(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, net.ErrClosed)
}

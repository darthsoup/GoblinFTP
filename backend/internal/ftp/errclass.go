package ftp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"path"
	"strings"
	"syscall"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// FTP reply codes that carry a meaning the API reports differently. jlaffaye
// surfaces these as *textproto.Error wherever it passes an expected code.
const (
	replyServiceNotAvailable = 421
	replyCantOpenData        = 425
	replyTransferAborted     = 426
	replyDataConnClosed      = 522
	replyFileUnavailable     = 550
	replyPageTypeUnknown     = 551
	replyExceededStorage     = 552
	replyFileNameNotAllowed  = 553
	replyNotLoggedIn         = 530
	replyNeedAccount         = 532
	replyBadSequence         = 503
)

// wrapErr maps a jlaffaye/ftp error onto a transfer sentinel. Matching the
// numeric reply beats substring matching, which false-positives on remote paths
// that happen to contain "552" or "permission denied".
func wrapErr(err error) error {
	if err == nil {
		return nil
	}

	var te *textproto.Error
	if errors.As(err, &te) {
		switch te.Code {
		case replyServiceNotAvailable:
			return fmt.Errorf("%w: %w", transfer.ErrConnectionLost, err)
		case replyCantOpenData, replyTransferAborted, replyDataConnClosed:
			return fmt.Errorf("%w: %w", transfer.ErrDataConnectionFailed, err)
		case replyExceededStorage:
			return fmt.Errorf("%w: %w", transfer.ErrQuotaExceeded, err)
		case replyFileNameNotAllowed:
			return fmt.Errorf("%w: %w", transfer.ErrPermissionDenied, err)
		case replyNotLoggedIn, replyNeedAccount:
			return fmt.Errorf("%w: %w", transfer.ErrAuthFailed, err)
		case replyPageTypeUnknown, replyBadSequence:
			return fmt.Errorf("%w: %w", transfer.ErrInvalidType, err)
		case replyFileUnavailable:
			// 550 is the universal refusal: only the server's own wording
			// separates "no such file" from "permission denied" from "in use".
			return wrap550(err, te.Msg)
		}
		return err
	}

	if isTimeout(err) {
		return fmt.Errorf("%w: %w", transfer.ErrConnectionTimeout, err)
	}
	if isSocketLoss(err) {
		return fmt.Errorf("%w: %w", transfer.ErrConnectionLost, err)
	}
	if isDataConnFailure(err) {
		return fmt.Errorf("%w: %w", transfer.ErrDataConnectionFailed, err)
	}
	return err
}

// wrap550 inspects only the server's reply text, never the full error string,
// so a remote path in the wrapper cannot steer the classification.
func wrap550(err error, msg string) error {
	m := strings.ToLower(msg)
	switch {
	case contains(m, "not empty", "directory not empty", "remove directory operation failed"):
		return fmt.Errorf("%w: %w", transfer.ErrDirNotEmpty, err)
	case contains(m, "permission denied", "access denied", "access is denied"):
		return fmt.Errorf("%w: %w", transfer.ErrPermissionDenied, err)
	case contains(m, "no such file", "no such directory", "not found", "does not exist"):
		return fmt.Errorf("%w: %w", transfer.ErrNotFound, err)
	case contains(m, "already exists", "file exists"):
		return fmt.Errorf("%w: %w", transfer.ErrFileExists, err)
	case contains(m, "quota", "disk full", "no space"):
		return fmt.Errorf("%w: %w", transfer.ErrQuotaExceeded, err)
	}
	return err
}

func contains(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func isTimeout(err error) bool {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func isSocketLoss(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, net.ErrClosed)
}

// isDataConnFailure catches the EPSV/PASV negotiation failures jlaffaye reports
// as plain strings, which are the usual symptom of NAT or a closed port range.
func isDataConnFailure(err error) bool {
	m := strings.ToLower(err.Error())
	return contains(m, "invalid epsv response format", "invalid pasv response format",
		"invalid port response format", "cannot open data connection")
}

// isRootPath guards the recursive delete: RemoveDirRecur("/") wipes the account.
func isRootPath(p string) bool {
	cleaned := path.Clean(p)
	return cleaned == "/" || cleaned == "." || cleaned == ""
}

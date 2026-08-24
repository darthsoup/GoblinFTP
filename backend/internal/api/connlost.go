package api

import (
	"errors"
	"io"
	"net"
	"os"
	"syscall"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// isConnLost reports whether err indicates the FTP/SFTP connection died. A false
// positive is expensive: it closes a live client and shows the reconnect dialog.
func isConnLost(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, transfer.ErrConnectionLost) {
		return true
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) {
		return true
	}
	// Past this point only wording is left, and wording is attacker-influenced:
	// a *os.PathError renders the remote path, so skip the text rules entirely
	// rather than let a filename named "connection lost" close a live session.
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return false
	}
	// Read the innermost error only, so an outer wrapper carrying a path cannot
	// smuggle one of these phrases into the match either.
	msg := deepestMessage(err)
	return contains(msg, "broken pipe", "connection reset",
		"use of closed network connection", "connection lost")
}

// failClient turns a transfer.Client error into an API failure. A lost connection
// closes and clears the client, returning ERR_CONNECTION_LOST, the SPA's reconnect cue.
func failClient(c echo.Context, code gftperrors.Code, err error) error {
	if isConnLost(err) {
		if sess, ok := c.Get("session").(*auth.Session); ok {
			// The caller already holds the transfer lock (non-reentrant), so this
			// Close is serialized with the in-flight transfer and must not retake it.
			if clientVal, ok := sess.Get("client"); ok {
				if client, ok := clientVal.(transfer.Client); ok {
					_ = client.Close()
					sess.Delete("client")
				}
			}
		}
		return Fail(c, gftperrors.New(gftperrors.ErrConnectionLost, "connection to the server was lost").WithCause(err))
	}
	// Map the raw protocol error to a stable code plus friendly message so strings
	// like `550 "..."` never reach the client. Unrecognized ones keep the caller's
	// code, and take that code's own message so the two can never disagree.
	classified, msg := classify(err)
	if classified == gftperrors.ErrOperationFailed && code != "" {
		classified = code
		msg = messageFor(code)
	}
	return Fail(c, gftperrors.New(classified, msg).WithCause(err))
}

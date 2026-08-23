package api

import (
	"errors"
	"io"
	"net"
	"strings"
	"syscall"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// isConnLost reports whether err indicates the FTP/SFTP connection died.
func isConnLost(err error) bool {
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) {
		return true
	}
	// jlaffaye/ftp and pkg/sftp wrap some socket failures in plain strings.
	msg := err.Error()
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "connection lost")
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
	// like `550 "..."` never reach the client. Unrecognized ones keep the caller's code.
	classified, msg := classify(err)
	if classified == gftperrors.ErrOperationFailed && code != "" {
		classified = code
	}
	return Fail(c, gftperrors.New(classified, msg).WithCause(err))
}

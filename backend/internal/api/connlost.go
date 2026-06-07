// backend/internal/api/connlost.go
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

// failClient converts a transfer.Client error into an API failure. When the
// error means the server connection died, the dead client is closed, removed
// from the session, and reported as ERR_CONNECTION_LOST with a clean message
// instead of a raw socket error — the frontend switches to its reconnect flow
// on that code.
func failClient(c echo.Context, code gftperrors.Code, err error) error {
	if isConnLost(err) {
		if sess, ok := c.Get("session").(*auth.Session); ok {
			if client, ok := sess.Data["client"].(transfer.Client); ok {
				_ = client.Close()
				delete(sess.Data, "client")
			}
		}
		return Fail(c, gftperrors.New(gftperrors.ErrConnectionLost, "connection to the server was lost").WithCause(err))
	}
	return Fail(c, gftperrors.New(code, err.Error()).WithCause(err))
}

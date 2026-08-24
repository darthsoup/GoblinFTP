package ftp

import (
	"net"
	"net/textproto"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// Retr returns (*Response, error), so returning it directly turned a nil
// *Response into a non-nil io.ReadCloser whose Close panicked.
func TestDownloadReturnsNilReaderOnError(t *testing.T) {
	srv := newRefusingServer(t)
	defer srv.Close()

	c, err := Dial(srv.Addr(), "user", "pass", true, nil)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	r, err := c.Download("/nope.txt")
	require.Error(t, err)
	assert.Nil(t, r, "a failed Download must yield a nil reader, not a nil pointer in an interface")

	// The old shape panicked here rather than returning an error.
	if r != nil {
		assert.NotPanics(t, func() { _ = r.Close() })
	}
}

func TestWrapErrMapsReplyCodes(t *testing.T) {
	tests := []struct {
		code int
		msg  string
		want error
	}{
		{421, "Service not available", transfer.ErrConnectionLost},
		{425, "Can't open data connection", transfer.ErrDataConnectionFailed},
		{426, "Transfer aborted", transfer.ErrDataConnectionFailed},
		{552, "Exceeded storage allocation", transfer.ErrQuotaExceeded},
		{553, "File name not allowed", transfer.ErrPermissionDenied},
		{530, "Not logged in", transfer.ErrAuthFailed},
		{550, "No such file or directory", transfer.ErrNotFound},
		{550, "Permission denied", transfer.ErrPermissionDenied},
		{550, "Directory not empty", transfer.ErrDirNotEmpty},
		{550, "File already exists", transfer.ErrFileExists},
	}
	for _, tt := range tests {
		got := wrapErr(&textproto.Error{Code: tt.code, Msg: tt.msg})
		assert.ErrorIs(t, got, tt.want, "%d %s", tt.code, tt.msg)
	}
}

// A path is server-controlled, so it must never steer the classification.
func TestWrapErrIgnoresPathTextInReplyCode(t *testing.T) {
	err := wrapErr(&textproto.Error{Code: 550, Msg: `/reports/2553/q1.csv: No such file`})
	assert.ErrorIs(t, err, transfer.ErrNotFound)
	assert.NotErrorIs(t, err, transfer.ErrPermissionDenied)
}

// jftp only bounds the TCP connect, so a server that accepts and never speaks
// used to block forever and wedge the session behind its transfer lock.
func TestDialBoundsSilentServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		// Accept and stay silent: never send the 220 greeting.
		defer func() { _ = conn.Close() }()
		time.Sleep(30 * time.Second)
	}()

	done := make(chan error, 1)
	go func() {
		_, dialErr := Dial(ln.Addr().String(), "u", "p", true, nil)
		done <- dialErr
	}()

	select {
	case dialErr := <-done:
		require.Error(t, dialErr)
		assert.ErrorIs(t, dialErr, transfer.ErrConnectionTimeout,
			"a silent server must be reported as a timeout, got %v", dialErr)
	case <-time.After(dialTimeout + 10*time.Second):
		t.Fatal("Dial never returned: the control connection is still unbounded")
	}
}

// refusingServer speaks just enough FTP to authenticate and then refuse RETR.
type refusingServer struct {
	ln net.Listener
}

func newRefusingServer(t *testing.T) *refusingServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &refusingServer{ln: ln}
	go s.serve()
	return s
}

func (s *refusingServer) Addr() string { return s.ln.Addr().String() }
func (s *refusingServer) Close()       { _ = s.ln.Close() }

func (s *refusingServer) serve() {
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	tp := textproto.NewConn(conn)
	_ = tp.PrintfLine("220 ready")
	for {
		line, readErr := tp.ReadLine()
		if readErr != nil {
			return
		}
		switch {
		case hasPrefix(line, "USER"):
			_ = tp.PrintfLine("331 need password")
		case hasPrefix(line, "PASS"):
			_ = tp.PrintfLine("230 logged in")
		case hasPrefix(line, "RETR"):
			_ = tp.PrintfLine("550 Permission denied")
		case hasPrefix(line, "EPSV"), hasPrefix(line, "PASV"):
			_ = tp.PrintfLine("550 Permission denied")
		case hasPrefix(line, "QUIT"):
			_ = tp.PrintfLine("221 bye")
			return
		default:
			_ = tp.PrintfLine("200 ok")
		}
	}
}

func hasPrefix(line, prefix string) bool {
	return len(line) >= len(prefix) && line[:len(prefix)] == prefix
}

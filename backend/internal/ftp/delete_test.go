package ftp

import (
	"errors"
	"io"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jftp "github.com/jlaffaye/ftp"
	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestDeleteRejectsInvalidPathsBeforeConnecting(t *testing.T) {
	for _, p := range []string{"/", "/child/..", "..", "../..", "relative", `/dir\..`} {
		assert.ErrorIs(t, (&Client{}).Delete(p), transfer.ErrInvalidType)
	}
}

func TestDeleteStopsServerControlledRecursion(t *testing.T) {
	control, peer := net.Pipe()
	defer control.Close()
	defer peer.Close()
	require.NoError(t, control.SetDeadline(time.Now().Add(3*time.Second)))
	tp := textproto.NewConn(peer)
	var replies sync.Mutex
	reply := func(format string, args ...any) error {
		replies.Lock()
		defer replies.Unlock()
		return tp.PrintfLine(format, args...)
	}
	commands := make(chan string, 512)
	var listings atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = reply("220 ready")
		for {
			line, err := tp.ReadLine()
			if err != nil {
				return
			}
			commands <- line
			command, _, _ := strings.Cut(line, " ")
			switch command {
			case "EPSV":
				_ = reply("229 Entering Extended Passive Mode (|||2121|)")
			case "LIST":
				listings.Add(1)
				_ = reply("150 data")
			case "QUIT":
				_ = reply("221 bye")
				return
			default:
				_ = reply("500 unsupported")
			}
		}
	}()
	first := true
	conn, err := jftp.Dial("127.0.0.1:21", jftp.DialWithDialFunc(func(string, string) (net.Conn, error) {
		if first {
			first = false
			return auditTCPConn{control}, nil
		}
		data, remote := net.Pipe()
		go func() {
			_, _ = io.WriteString(remote, "drwxr-xr-x 1 u g 0 Oct 19 12:00 r\r\n")
			_ = remote.Close()
			_ = reply("226 complete")
		}()
		return data, nil
	}))
	require.NoError(t, err)
	client := &Client{conn: conn}
	err = client.Delete("/r")
	if !errors.Is(err, transfer.ErrInvalidType) {
		_ = control.Close()
		<-done
		close(commands)
		var seen []string
		for c := range commands {
			seen = append(seen, c)
		}
		t.Fatalf("delete: %v, commands: %v", err, seen)
	}
	require.LessOrEqual(t, listings.Load(), int32(130))
	require.NoError(t, client.Close())
	<-done
}

type auditTCPConn struct{ net.Conn }

func (c auditTCPConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 21}
}

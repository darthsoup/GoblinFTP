package ftp

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDataInactivityClosesSocket(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	conn := &deadlineConn{Conn: a}
	conn.setTimeout(20 * time.Millisecond)
	start := time.Now()
	_, err := conn.Read(make([]byte, 1))
	require.Error(t, err)
	require.True(t, isTimeout(err))
	require.Less(t, time.Since(start), time.Second)
	_, err = b.Write([]byte("x"))
	require.Error(t, err)
}

func TestCompletionDeadlineCannotBeExtendedByRollingDeadline(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	defer a.Close()
	conn := &deadlineConn{Conn: a}
	conn.setTimeout(time.Minute)
	require.NoError(t, conn.SetDeadline(time.Now().Add(20*time.Millisecond)))
	start := time.Now()
	_, err := conn.Read(make([]byte, 1))
	require.Error(t, err)
	require.Less(t, time.Since(start), time.Second)
}

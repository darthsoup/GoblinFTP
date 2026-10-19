package ftp

import (
	"errors"
	"io"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestListWithLimitsStopsInsideParser(t *testing.T) {
	for _, limits := range [][2]int{{2, 4096}, {100, 20}} {
		control, peer := net.Pipe()
		data, dataPeer := net.Pipe()
		t.Cleanup(func() { _ = control.Close(); _ = peer.Close(); _ = data.Close(); _ = dataPeer.Close() })
		_ = control.SetDeadline(time.Now().Add(time.Second))
		_ = peer.SetDeadline(time.Now().Add(time.Second))
		serverDone := make(chan struct{})
		go func() {
			defer close(serverDone)
			tp := textproto.NewConn(peer)
			_, _ = tp.ReadLine()
			_ = tp.PrintfLine("229 Entering Extended Passive Mode (|||2121|)")
			_, _ = tp.ReadLine()
			_ = tp.PrintfLine("150 opening")
			_, _ = io.WriteString(dataPeer, strings.Repeat("-rw-r--r-- 1 u g 1 Oct 19 12:00 file.txt\r\n", 4))
			_ = dataPeer.Close()
		}()
		client := &ServerConn{conn: textproto.NewConn(control), netConn: control, host: "127.0.0.1", options: &dialOptions{dialFunc: func(string, string) (net.Conn, error) { return data, nil }, location: time.UTC}}
		entries, err := client.ListWithLimits("/", limits[0], limits[1])
		if !errors.Is(err, ErrListingLimit) || entries != nil {
			t.Fatalf("got %v, %v", entries, err)
		}
		<-serverDone
	}
}

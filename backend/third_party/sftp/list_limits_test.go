package sftp

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestReadDirWithLimitsStopsInsideParser(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, limits := range [][2]int{{2, 4096}, {100, 20}} {
		a, b := net.Pipe()
		server, err := NewServer(a)
		if err != nil {
			t.Fatal(err)
		}
		go func() { _ = server.Serve() }()
		client, err := NewClientPipe(b, b)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := client.ReadDirWithLimits(context.Background(), dir, limits[0], limits[1])
		_ = client.Close()
		_ = server.Close()
		if !errors.Is(err, ErrListingLimit) || entries != nil {
			t.Fatalf("got %v, %v", entries, err)
		}
	}
}

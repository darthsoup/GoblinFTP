package sftp

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	psftp "github.com/pkg/sftp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

func deleteTestClient(t *testing.T) *Client {
	t.Helper()
	a, b := net.Pipe()
	server, err := psftp.NewServer(a)
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve()
	}()
	client, err := psftp.NewClientPipe(b, b)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		<-done
	})
	return &Client{sftp: client}
}

func TestDeleteRejectsInvalidPathsBeforeConnecting(t *testing.T) {
	for _, p := range []string{"/", "/child/..", "..", "../..", "relative"} {
		assert.ErrorIs(t, (&Client{}).Delete(p), transfer.ErrInvalidType)
	}
}

func TestDeleteDoesNotFollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the in-process SFTP fixture requires POSIX filesystem paths")
	}
	for _, kind := range []string{"file", "directory", "broken", "nested"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "target")
			require.NoError(t, os.Mkdir(target, 0o700))
			keep := filepath.Join(target, "keep.txt")
			require.NoError(t, os.WriteFile(keep, []byte("keep"), 0o600))
			selected := filepath.Join(root, "selected")
			link := selected
			linkTarget := target
			switch kind {
			case "file":
				linkTarget = keep
			case "broken":
				linkTarget = filepath.Join(root, "missing")
			case "nested":
				require.NoError(t, os.Mkdir(selected, 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(selected, "remove.txt"), []byte("remove"), 0o600))
				link = filepath.Join(selected, "link")
			}
			linkErr := os.Symlink(linkTarget, link)
			if linkErr != nil && runtime.GOOS == "windows" {
				t.Skipf("symlink fixture requires Windows privileges: %v", linkErr)
			}
			require.NoError(t, linkErr)
			client := deleteTestClient(t)
			require.NoError(t, client.Delete(selected))
			_, err := os.Lstat(selected)
			assert.True(t, os.IsNotExist(err), "selected object remains: %v", err)
			got, err := os.ReadFile(keep)
			require.NoError(t, err)
			assert.Equal(t, "keep", string(got))
		})
	}
}

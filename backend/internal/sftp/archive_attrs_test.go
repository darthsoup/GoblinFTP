package sftp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"io/fs"
	"net"
	"testing"
	"testing/fstest"
	"time"

	psftp "github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
)

func TestArchiveRequiresExplicitOwnerAndPermissions(t *testing.T) {
	for _, flags := range []uint32{0, 2, 4, 6} {
		var pkt [13]byte
		binary.BigEndian.PutUint32(pkt[:], 9)
		pkt[4] = 105
		binary.BigEndian.PutUint32(pkt[9:], flags)
		_, err := io.ReadAll(&archiveReplies{r: bufio.NewReader(bytes.NewReader(pkt[:]))})
		if flags == 6 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestCanceledListReleasesStalledSFTPRequest(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	defer local.Close()
	ready := make(chan struct{})
	go func() {
		var init [9]byte
		if _, err := io.ReadFull(remote, init[:]); err != nil {
			return
		}
		init[4] = 2
		if _, err := remote.Write(init[:]); err != nil {
			return
		}
		var header [4]byte
		if _, err := io.ReadFull(remote, header[:]); err != nil {
			return
		}
		if _, err := io.CopyN(io.Discard, remote, int64(binary.BigEndian.Uint32(header[:]))); err != nil {
			return
		}
		close(ready)
	}()
	sub, err := psftp.NewClientPipe(local, local)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &Client{sftp: sub, raw: &activityConn{Conn: local}, ctx: ctx}
	returned := make(chan error, 1)
	go func() { _, err := c.List("/"); returned <- err }()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("listing never reached server")
	}
	cancel()
	select {
	case err := <-returned:
		require.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("listing did not release caller")
	}
}

func TestArchiveDirectoryRequiresExclusiveWriteAuthority(t *testing.T) {
	target := archiveTarget{uid: 42}
	for _, tc := range []struct {
		mode fs.FileMode
		uid  uint32
		safe bool
	}{
		{fs.ModeDir | 0755, 0, true}, {fs.ModeDir | 0700, 42, true}, {fs.ModeDir | 0775, 42, false},
		{fs.ModeDir | 0777, 0, false}, {fs.ModeDir | 0700, 43, false}, {fs.ModeSymlink | 0700, 42, false},
	} {
		tree := fstest.MapFS{"entry": {Mode: tc.mode, Sys: &psftp.FileStat{UID: tc.uid}}}
		info, err := tree.Stat("entry")
		require.NoError(t, err)
		require.Equal(t, tc.safe, target.trustedDirectory(info))
	}
}

package sftp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	pkgsftp "github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

type pendingDeadlineContext struct {
	context.Context
	deadline time.Time
}

func (c pendingDeadlineContext) Deadline() (time.Time, bool) {
	return c.deadline, true
}

func TestSetupErrorRecognizesElapsedDeadlineBeforeContextTimer(t *testing.T) {
	ctx := pendingDeadlineContext{Context: context.Background(), deadline: time.Now().Add(-time.Second)}
	require.NoError(t, ctx.Err())
	err := setupError(ctx, io.EOF, transfer.ErrSubsystemUnavailable)
	require.ErrorIs(t, err, transfer.ErrConnectionTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestDialContextBoundsSetupStages(t *testing.T) {
	for _, stage := range []string{"greeting", "authentication", "subsystem", "version", "working-directory"} {
		t.Run(stage, func(t *testing.T) {
			addr, kh, _ := setupSSHServer(t, stage)
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			start := time.Now()
			client, prompt, err := DialContext(ctx, addr, "u", "p", "", kh)
			require.Nil(t, client)
			require.Nil(t, prompt)
			require.ErrorIs(t, err, transfer.ErrConnectionTimeout)
			require.Less(t, time.Since(start), 2*time.Second)
		})
	}
}

func TestDialContextCancellationClosesPeer(t *testing.T) {
	addr, kh, closed := setupSSHServer(t, "greeting")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, _, err := DialContext(ctx, addr, "u", "p", "", kh); result <- err }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("canceled setup did not return")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("canceled setup left peer connected")
	}
}

func TestDialContextClearsSetupDeadline(t *testing.T) {
	addr, kh, _ := setupSSHServer(t, "ready")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	client, prompt, err := DialContext(ctx, addr, "u", "p", "", kh)
	require.NoError(t, err)
	require.Nil(t, prompt)
	defer func() { _ = client.Close() }()
	require.NotEmpty(t, client.initialDir)
	time.Sleep(350 * time.Millisecond)
	require.NoError(t, client.Ping(), "completed session must survive its setup context")
}

func setupSSHServer(t *testing.T, stage string) (string, string, <-chan struct{}) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(private)
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	stopped := make(chan struct{})
	t.Cleanup(func() { close(stopped); _ = listener.Close() })
	kh := filepath.Join(t.TempDir(), "known_hosts")
	_, err = loadKnownHosts(kh)
	require.NoError(t, err)
	require.NoError(t, appendKnownHost(kh, listener.Addr().String(), signer.PublicKey()))
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		raw, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		go func() { <-stopped; _ = raw.Close() }()
		if stage == "greeting" {
			_, _ = io.Copy(io.Discard, raw)
			return
		}
		cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			if stage == "authentication" {
				<-stopped
				return nil, errors.New("stopped")
			}
			return nil, nil
		}}
		cfg.AddHostKey(signer)
		conn, channels, requests, handshakeErr := ssh.NewServerConn(raw, cfg)
		if handshakeErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			channel, channelRequests, channelErr := incoming.Accept()
			if channelErr != nil {
				return
			}
			for request := range channelRequests {
				if request.Type != "subsystem" {
					_ = request.Reply(false, nil)
					continue
				}
				if stage == "subsystem" {
					_, _ = io.Copy(io.Discard, channel)
					return
				}
				_ = request.Reply(true, nil)
				if stage == "version" {
					_, _ = io.Copy(io.Discard, channel)
					return
				}
				if stage == "working-directory" {
					var header [4]byte
					if _, err = io.ReadFull(channel, header[:]); err != nil {
						return
					}
					if _, err = io.CopyN(io.Discard, channel, int64(binary.BigEndian.Uint32(header[:]))); err != nil {
						return
					}
					_, _ = channel.Write([]byte{0, 0, 0, 5, 2, 0, 0, 0, 3})
					_, _ = io.Copy(io.Discard, channel)
					return
				}
				server, serverErr := pkgsftp.NewServer(channel)
				if serverErr != nil {
					return
				}
				_ = server.Serve()
				_ = server.Close()
				return
			}
		}
	}()
	return listener.Addr().String(), kh, closed
}

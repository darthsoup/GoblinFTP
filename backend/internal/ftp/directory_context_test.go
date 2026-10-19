package ftp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

type directoryServerOptions struct {
	mlsd           bool
	count          int
	lookupAt       int
	metadataType   string
	stall          string
	completion     string
	entry          func(int) string
	startOperation chan struct{}
}

type directoryServer struct {
	listener net.Listener
	options  directoryServerOptions
	sent     atomic.Int32
	mu       sync.Mutex
	conns    []net.Conn
	commands []string
	data     net.Listener
	started  sync.Once
}

func newDirectoryServer(t *testing.T, options directoryServerOptions) *directoryServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &directoryServer{listener: listener, options: options}
	t.Cleanup(s.close)
	go s.serve()
	return s
}

func (s *directoryServer) close() {
	_ = s.listener.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data != nil {
		_ = s.data.Close()
	}
	for _, conn := range s.conns {
		_ = conn.Close()
	}
}

func (s *directoryServer) signalOperation() {
	if s.options.startOperation != nil {
		s.started.Do(func() { close(s.options.startOperation) })
	}
}

func (s *directoryServer) serve() {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	s.mu.Lock()
	s.conns = append(s.conns, conn)
	s.mu.Unlock()
	defer func() { _ = conn.Close() }()
	control := textproto.NewConn(conn)
	_ = control.PrintfLine("220 ready")
	for {
		line, err := control.ReadLine()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.commands = append(s.commands, line)
		s.mu.Unlock()
		switch {
		case strings.HasPrefix(line, "USER"):
			_ = control.PrintfLine("331 password")
		case strings.HasPrefix(line, "PASS"):
			_ = control.PrintfLine("230 logged in")
		case line == "FEAT" && s.options.mlsd:
			_ = control.PrintfLine("211-Features")
			_ = control.PrintfLine(" MLST type*;size*;modify*;")
			_ = control.PrintfLine("211 End")
		case line == "FEAT":
			_ = control.PrintfLine("211 no features")
		case line == "EPSV":
			if s.options.stall == "passive" {
				s.signalOperation()
				_, _ = io.Copy(io.Discard, conn)
				return
			}
			data, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return
			}
			s.mu.Lock()
			s.data = data
			s.mu.Unlock()
			_ = control.PrintfLine("229 Entering Extended Passive Mode (|||%d|)", data.Addr().(*net.TCPAddr).Port)
		case strings.HasPrefix(line, "MLST"):
			s.signalOperation()
			if s.options.stall == "metadata" {
				_, _ = io.Copy(io.Discard, conn)
				return
			}
			kind := s.options.metadataType
			if kind == "" {
				kind = "dir"
			}
			_ = control.PrintfLine("250-File details")
			_ = control.PrintfLine(" type=%s;size=9;modify=20261019000000; target", kind)
			_ = control.PrintfLine("250 End")
		case strings.HasPrefix(line, "MLSD"), strings.HasPrefix(line, "LIST"):
			s.signalOperation()
			if s.options.stall == "listing-command" {
				_, _ = io.Copy(io.Discard, conn)
				return
			}
			_ = control.PrintfLine("150 opening data")
			data, err := s.data.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns = append(s.conns, data)
			s.mu.Unlock()
			if s.options.stall == "data" {
				_, _ = io.Copy(io.Discard, data)
			} else {
				writer := bufio.NewWriterSize(data, 1024)
				for i := range s.options.count {
					entry := s.listingLine(i)
					if _, err := writer.WriteString(entry + "\r\n"); err != nil {
						break
					}
					s.sent.Add(1)
				}
				_ = writer.Flush()
			}
			_ = data.Close()
			_ = s.data.Close()
			if s.options.completion == "missing" {
				_, _ = io.Copy(io.Discard, conn)
				return
			}
			if s.options.completion == "aborted" {
				_ = control.PrintfLine("426 data stopped")
			}
			_ = control.PrintfLine("226 complete")
		case strings.HasPrefix(line, "RETR"):
			_ = control.PrintfLine("150 opening data")
			data, err := s.data.Accept()
			if err != nil {
				return
			}
			_, _ = io.WriteString(data, "ordinary download")
			_ = data.Close()
			_ = s.data.Close()
			_ = control.PrintfLine("226 complete")
		case line == "QUIT":
			_ = control.PrintfLine("221 bye")
			return
		default:
			_ = control.PrintfLine("200 ok")
		}
	}
}

func (s *directoryServer) listingLine(i int) string {
	if s.options.entry != nil {
		return s.options.entry(i)
	}
	name := fmt.Sprintf("file-%d", i)
	if i == s.options.lookupAt {
		name = "target"
	}
	if s.options.mlsd {
		return fmt.Sprintf("type=file;size=1;modify=20261019000000; %s", name)
	}
	return fmt.Sprintf("-rw-r--r-- 1 owner group 1 Oct 19 2026 %s", name)
}

func directoryClient(t *testing.T, server *directoryServer) *Client {
	t.Helper()
	client, err := Dial(server.listener.Addr().String(), "user", "pass", true, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestReadDirContextBoundsLargeListingAndKeepsConnection(t *testing.T) {
	for _, mlsd := range []bool{false, true} {
		t.Run(fmt.Sprintf("mlsd=%t", mlsd), func(t *testing.T) {
			server := newDirectoryServer(t, directoryServerOptions{mlsd: mlsd, count: 1000000, lookupAt: -1, completion: "aborted"})
			client := directoryClient(t, server)
			entries, truncated, err := client.ReadDirContext(context.Background(), "/large", 3)
			require.NoError(t, err)
			assert.Len(t, entries, 3)
			assert.True(t, truncated)
			assert.Less(t, server.sent.Load(), int32(1000000))
			require.NoError(t, client.Ping())
			server.mu.Lock()
			commands := append([]string(nil), server.commands...)
			server.mu.Unlock()
			if mlsd {
				assert.Contains(t, commands, "MLSD /large")
			} else {
				assert.Contains(t, commands, "LIST -a /large")
			}
		})
	}
}

func TestReadDirContextCompleteListing(t *testing.T) {
	server := newDirectoryServer(t, directoryServerOptions{count: 3, lookupAt: -1})
	client := directoryClient(t, server)
	entries, truncated, err := client.ReadDirContext(context.Background(), "/small", 3)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
	assert.False(t, truncated)
	require.NoError(t, client.Ping())
}

func TestReadDirContextDotEntriesDoNotConsumeLimit(t *testing.T) {
	for _, mlsd := range []bool{false, true} {
		for _, fileCount := range []int{2, 3} {
			t.Run(fmt.Sprintf("mlsd=%t/files=%d", mlsd, fileCount), func(t *testing.T) {
				server := newDirectoryServer(t, directoryServerOptions{
					mlsd: mlsd, count: fileCount + 2,
					entry: func(i int) string {
						name := fmt.Sprintf("file-%d", i)
						kind := "file"
						if i < 2 {
							name = []string{".", ".."}[i]
							kind = []string{"cdir", "pdir"}[i]
						}
						if mlsd {
							return fmt.Sprintf("type=%s;size=1;modify=20261019000000; %s", kind, name)
						}
						mode := "-rw-r--r--"
						if i < 2 {
							mode = "drwxr-xr-x"
						}
						return fmt.Sprintf("%s 1 owner group 1 Oct 19 2026 %s", mode, name)
					},
				})
				client := directoryClient(t, server)
				entries, truncated, err := client.ReadDirContext(context.Background(), "/directory", 2)
				require.NoError(t, err)
				require.Len(t, entries, 2)
				assert.Equal(t, "file-2", entries[0].Name)
				assert.Equal(t, "file-3", entries[1].Name)
				assert.Equal(t, fileCount > 2, truncated)
				require.NoError(t, client.Ping())
			})
		}
	}
}

func TestReadDirContextMalformedAndDuplicateDotWorkIsBounded(t *testing.T) {
	for _, entry := range []string{"unsupported listing line", "drwxr-xr-x 1 owner group 1 Oct 19 2026 ."} {
		t.Run(entry, func(t *testing.T) {
			server := newDirectoryServer(t, directoryServerOptions{
				count: 1000000, completion: "aborted", entry: func(int) string { return entry },
			})
			client := directoryClient(t, server)
			entries, truncated, err := client.ReadDirContext(context.Background(), "/invalid", 2)
			require.Error(t, err)
			assert.Empty(t, entries)
			assert.True(t, truncated)
			assert.Less(t, server.sent.Load(), int32(1000000))
			require.NoError(t, client.Ping())
		})
	}
}

func TestDirectoryScanCancellationClosesUncertainConnection(t *testing.T) {
	for _, stall := range []string{"metadata", "passive", "listing-command", "data"} {
		t.Run(stall, func(t *testing.T) {
			started := make(chan struct{})
			server := newDirectoryServer(t, directoryServerOptions{mlsd: true, stall: stall, startOperation: started})
			client := directoryClient(t, server)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			results := make(chan error, 1)
			go func() {
				if stall == "metadata" {
					_, err := client.LstatContext(ctx, "/target")
					results <- err
					return
				}
				_, _, err := client.ReadDirContext(ctx, "/stalled", 5)
				results <- err
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("scan did not start")
			}
			cancel()
			select {
			case err := <-results:
				require.ErrorIs(t, err, context.Canceled)
				require.ErrorIs(t, err, transfer.ErrConnectionLost)
			case <-time.After(2 * time.Second):
				t.Fatal("scan ignored cancellation")
			}
		})
	}
}

func TestReadDirContextCannotReuseMissingCompletion(t *testing.T) {
	server := newDirectoryServer(t, directoryServerOptions{count: 2, lookupAt: -1, completion: "missing"})
	client := directoryClient(t, server)
	started := time.Now()
	_, _, err := client.ReadDirContext(context.Background(), "/small", 3)
	require.ErrorIs(t, err, transfer.ErrConnectionLost)
	assert.Less(t, time.Since(started), 3*time.Second)
}

func TestDirectoryScanDeadlineInterruptsData(t *testing.T) {
	server := newDirectoryServer(t, directoryServerOptions{mlsd: true, stall: "data"})
	client := directoryClient(t, server)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _, err := client.ReadDirContext(ctx, "/stalled", 5)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, transfer.ErrConnectionLost)
	assert.Less(t, time.Since(started), time.Second)
}

func TestCompletedScanCancellationDoesNotAffectDownload(t *testing.T) {
	server := newDirectoryServer(t, directoryServerOptions{mlsd: true, count: 1})
	client := directoryClient(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	_, truncated, err := client.ReadDirContext(ctx, "/small", 3)
	require.NoError(t, err)
	assert.False(t, truncated)
	cancel()
	r, err := client.Download("/file")
	require.NoError(t, err)
	content, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Equal(t, "ordinary download", string(content))
	require.NoError(t, client.Ping())
}

func TestPreCanceledDirectoryReadKeepsConnection(t *testing.T) {
	server := newDirectoryServer(t, directoryServerOptions{})
	client := directoryClient(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := client.ReadDirContext(ctx, "/unused", 3)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, transfer.ErrConnectionLost)
	require.NoError(t, client.Ping())
}

func TestLstatContextStreamsLegacyParentAndHonorsCap(t *testing.T) {
	for _, lookupAt := range []int{1, 50001} {
		t.Run(fmt.Sprintf("target=%d", lookupAt), func(t *testing.T) {
			server := newDirectoryServer(t, directoryServerOptions{count: 1000000, lookupAt: lookupAt, completion: "aborted"})
			client := directoryClient(t, server)
			info, err := client.LstatContext(context.Background(), "/target")
			if lookupAt == 1 {
				require.NoError(t, err)
				assert.Equal(t, "target", info.Name)
			} else {
				require.ErrorIs(t, err, transfer.ErrDirectorySizeLimit)
			}
			assert.Less(t, server.sent.Load(), int32(1000000))
			require.NoError(t, client.Ping())
		})
	}
}

func TestLstatContextUsesMLSTAndRecognizesSymlinks(t *testing.T) {
	server := newDirectoryServer(t, directoryServerOptions{mlsd: true, metadataType: "OS.unix=slink:/target"})
	client := directoryClient(t, server)
	info, err := client.LstatContext(context.Background(), "/link")
	require.NoError(t, err)
	assert.True(t, info.IsSymlink)
	server.mu.Lock()
	commands := append([]string(nil), server.commands...)
	server.mu.Unlock()
	assert.Contains(t, commands, "MLST /link")
	for _, command := range commands {
		assert.False(t, strings.HasPrefix(command, "MLSD") || strings.HasPrefix(command, "LIST"))
	}
	require.NoError(t, client.Ping())
}

func TestReadDirContextRecognizesMLSDSymlinks(t *testing.T) {
	server := newDirectoryServer(t, directoryServerOptions{mlsd: true, count: 2, entry: func(i int) string {
		if i == 0 {
			return "type=OS.unix=slink:/target;size=100;modify=20261019000000; link"
		}
		return "type=file;size=2;modify=20261019000000; actual"
	}})
	client := directoryClient(t, server)
	entries, truncated, err := client.ReadDirContext(context.Background(), "/small", 3)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.True(t, entries[0].IsSymlink)
	assert.False(t, entries[1].IsSymlink)
	assert.False(t, truncated)
}

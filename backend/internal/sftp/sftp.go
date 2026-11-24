package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// Client wraps pkg/sftp and implements transfer.Client.
type Client struct {
	hostFingerprint string
	archiveUID      *uint32
	raw             *activityConn
	ctx             context.Context
	ssh             *ssh.Client
	sftp            *sftp.Client
	initialDir      string
}

// Dial opens an SSH/SFTP session, verifying the host key against knownHostsPath. An
// unknown key requires confirmation. A changed pin requires operator intervention.
func Dial(addr, user, pass, acceptFingerprint, knownHostsPath string) (*Client, *HostKeyPrompt, error) {
	return DialContext(context.Background(), addr, user, pass, acceptFingerprint, knownHostsPath)
}

// DialContext bounds all setup round trips, including the initial directory lookup.
func DialContext(ctx context.Context, addr, user, pass, acceptFingerprint, knownHostsPath string) (*Client, *HostKeyPrompt, error) {
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	var res hostKeyResult
	cb, err := buildHostKeyCallback(addr, knownHostsPath, acceptFingerprint, &res)
	if err != nil {
		// A known_hosts file that cannot be read is an operator problem. Calling
		// it a connection failure sent the user to check the wrong thing.
		return nil, nil, fmt.Errorf("%w: %w", transfer.ErrHostKeyStoreUnavailable, err)
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: cb,
		Timeout:         dialTimeout,
	}
	raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, setupError(ctx, err, transfer.ErrConnectionFailed)
	}
	connected := false
	defer func() {
		if !connected {
			_ = raw.Close()
		}
	}()
	deadline, _ := ctx.Deadline()
	if err := raw.SetDeadline(deadline); err != nil {
		return nil, nil, setupError(ctx, err, transfer.ErrConnectionFailed)
	}
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stop()
	transport := &activityConn{Conn: raw}
	conn, channels, requests, err := ssh.NewClientConn(transport, addr, cfg)
	if err != nil {
		// Order matters: a failed known_hosts write reports "permission denied",
		// which isAuthErr would otherwise read as bad credentials and throttle
		// the user into a lockout over a read-only data volume.
		switch {
		case res.writeErr != nil:
			return nil, nil, fmt.Errorf("%w: %w", transfer.ErrHostKeyStoreUnavailable, res.writeErr)
		case res.mismatch:
			return nil, nil, fmt.Errorf("%w: host key changed; contact the administrator", transfer.ErrHostKeyMismatch)
		case res.revoked:
			return nil, nil, fmt.Errorf("%w: the server's host key is marked revoked in known_hosts",
				transfer.ErrHostKeyMismatch)
		case res.prompt != nil:
			return nil, res.prompt, nil // unknown or changed host key, needs confirmation
		case ctx.Err() != nil || isTimeout(err):
			return nil, nil, setupError(ctx, err, transfer.ErrConnectionFailed)
		case isAuthErr(err.Error()):
			return nil, nil, fmt.Errorf("%w: %w", transfer.ErrAuthFailed, err)
		default:
			return nil, nil, fmt.Errorf("%w: %w", transfer.ErrConnectionFailed, err)
		}
	}
	sshConn := ssh.NewClient(conn, channels, requests)
	sftpClient, err := sftp.NewClient(sshConn)
	if err != nil {
		_ = sshConn.Close()
		// The login worked, the server just does not offer the subsystem. Telling
		// the user "could not connect" sends them to check the wrong thing.
		return nil, nil, setupError(ctx, err, transfer.ErrSubsystemUnavailable)
	}
	initialDir, err := sftpClient.Getwd()
	if err != nil {
		_ = sshConn.Close()
		return nil, nil, setupError(ctx, err, transfer.ErrConnectionFailed)
	}
	if !stop() || ctx.Err() != nil {
		_ = sshConn.Close()
		return nil, nil, setupError(ctx, ctx.Err(), transfer.ErrConnectionFailed)
	}
	if err := raw.SetDeadline(time.Time{}); err != nil {
		_ = sshConn.Close()
		return nil, nil, setupError(ctx, err, transfer.ErrConnectionFailed)
	}
	connected = true
	return &Client{hostFingerprint: res.fingerprint, raw: transport, ctx: context.Background(), ssh: sshConn, sftp: sftpClient, initialDir: initialDir}, nil, nil
}

func setupError(ctx context.Context, err, fallback error) error {
	if cause := ctx.Err(); cause != nil {
		err = cause
	} else if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		// The socket deadline can fire before the context timer publishes its error.
		err = context.DeadlineExceeded
	}
	if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
		fallback = transfer.ErrConnectionTimeout
	}
	return fmt.Errorf("%w: %w", fallback, err)
}

const dialTimeout = 10 * time.Second

// WithContext scopes cancellation to requests using the serialized connection.
func (c Client) WithContext(ctx context.Context) transfer.Client {
	c.ctx = ctx
	return &c
}

func (c *Client) context() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func (c *Client) boundIO() func() {
	if c.raw == nil {
		return func() {}
	}
	return c.raw.begin(c.context())
}

type timedFile struct {
	file   *sftp.File
	client *Client
}

func (f timedFile) Read(p []byte) (int, error)  { defer f.client.boundIO()(); return f.file.Read(p) }
func (f timedFile) Write(p []byte) (int, error) { defer f.client.boundIO()(); return f.file.Write(p) }
func (f timedFile) Close() error                { defer f.client.boundIO()(); return f.file.Close() }

func (c *Client) WorkingDir() (string, error) {
	// The adapter never changes directories; setup already validated this lookup.
	return c.initialDir, nil
}

func (c *Client) List(dir string) ([]transfer.FileInfo, error) {
	defer c.boundIO()()
	entries, err := c.sftp.ReadDirWithLimits(c.context(), dir, 100000, 16<<20)
	if err != nil {
		return nil, wrapErr(err)
	}
	out := make([]transfer.FileInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, infoFromFS(e))
	}
	return out, nil
}

func (c *Client) Stat(p string) (transfer.FileInfo, error) {
	defer c.boundIO()()
	fi, err := c.sftp.Stat(p)
	if err != nil {
		return transfer.FileInfo{}, wrapErr(err)
	}
	return infoFromFS(fi), nil
}

func (c *Client) MakeDir(p string) error {
	defer c.boundIO()()
	return wrapErr(c.sftp.MkdirAll(p))
}

func (c *Client) Delete(p string) error {
	err := transfer.ValidateRemotePath(p, false, false)
	if err != nil {
		return err
	}
	budget := 100000
	return c.deleteBudget(p, 0, &budget)
}

func (c *Client) deleteBudget(p string, depth int, budget *int) error {
	*budget--
	if *budget < 0 {
		return transfer.ErrInvalidType
	}
	if depth > 64 {
		return fmt.Errorf("%w: directory tree is nested too deeply", transfer.ErrInvalidType)
	}
	done := c.boundIO()
	fi, err := c.sftp.Lstat(p)
	if err != nil {
		done()
		return wrapErr(err)
	}
	if !fi.IsDir() || fi.Mode()&fs.ModeSymlink != 0 {
		defer done()
		return wrapErr(c.sftp.Remove(p))
	}
	entries, err := c.sftp.ReadDirWithLimits(c.context(), p, 100000, 16<<20)
	done()
	if err != nil {
		return wrapErr(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." {
			continue
		}
		if transfer.ValidateChildName(name, false) != nil {
			return fmt.Errorf("%w: invalid remote directory entry", transfer.ErrInvalidType)
		}
		if err := c.deleteBudget(path.Join(p, name), depth+1, budget); err != nil {
			return err
		}
	}
	defer c.boundIO()()
	return wrapErr(c.sftp.RemoveDirectory(p))
}

func (c *Client) Rename(src, dst string) error {
	defer c.boundIO()()
	return wrapErr(c.sftp.Rename(src, dst))
}

// SupportsChmod is true because SFTP defines SETSTAT; an individual server may
// still refuse a given path, which surfaces as a normal error from Chmod.
func (c *Client) SupportsChmod() bool { return true }

func (c *Client) Chmod(p string, mode uint32) error {
	defer c.boundIO()()
	return wrapErr(c.sftp.Chmod(p, fs.FileMode(mode)))
}

func (c *Client) Download(p string) (io.ReadCloser, error) {
	defer c.boundIO()()
	fi, err := c.sftp.Stat(p)
	if err != nil {
		return nil, wrapErr(err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory", transfer.ErrInvalidType, p)
	}
	f, err := c.sftp.Open(p)
	if err != nil {
		return nil, wrapErr(err)
	}
	return timedFile{f, c}, nil
}

func (c *Client) Upload(p string, r io.Reader) error {
	done := c.boundIO()
	f, err := c.sftp.Create(p)
	done()
	if err != nil {
		return wrapErr(err)
	}
	return c.writeFile(f, r)
}

func (c *Client) writeFile(f *sftp.File, r io.Reader) error {
	// Close carries the server's status for the final flush, so discarding it
	// reports a quota or I/O failure at end-of-write as a successful upload.
	file := timedFile{f, c}
	_, copyErr := io.Copy(file, r)
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("%w: %w", transfer.ErrTransferIncomplete, wrapErr(copyErr))
	}
	if closeErr != nil {
		return fmt.Errorf("%w: %w", transfer.ErrTransferIncomplete, wrapErr(closeErr))
	}
	return nil
}

func (c *Client) Ping() error {
	defer c.boundIO()()
	_, err := c.sftp.Getwd()
	return wrapErr(err)
}

func (c *Client) Close() error {
	defer c.boundIO()()
	sftpErr := c.sftp.Close()
	sshErr := c.ssh.Close()
	if sftpErr != nil {
		return sftpErr
	}
	return sshErr
}

var _ transfer.Client = (*Client)(nil)

func infoFromFS(fi fs.FileInfo) transfer.FileInfo {
	return transfer.FileInfo{
		Name:        fi.Name(),
		Size:        fi.Size(),
		IsDir:       fi.IsDir(),
		ModTime:     fi.ModTime().Unix(),
		Permissions: fi.Mode().String(),
	}
}

func isAuthErr(msg string) bool {
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "auth fail")
}

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
	ssh        *ssh.Client
	sftp       *sftp.Client
	initialDir string
}

// Dial opens an SSH/SFTP session, verifying the host key against knownHostsPath. An
// unknown or changed key returns a *HostKeyPrompt and no client; re-Dial with acceptFingerprint to pin it.
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
	conn, channels, requests, err := ssh.NewClientConn(raw, addr, cfg)
	if err != nil {
		// Order matters: a failed known_hosts write reports "permission denied",
		// which isAuthErr would otherwise read as bad credentials and throttle
		// the user into a lockout over a read-only data volume.
		switch {
		case res.writeErr != nil:
			return nil, nil, fmt.Errorf("%w: %w", transfer.ErrHostKeyStoreUnavailable, res.writeErr)
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
	return &Client{ssh: sshConn, sftp: sftpClient, initialDir: initialDir}, nil, nil
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

func (c *Client) WorkingDir() (string, error) {
	// The adapter never changes directories; setup already validated this lookup.
	return c.initialDir, nil
}

func (c *Client) List(dir string) ([]transfer.FileInfo, error) {
	entries, err := c.sftp.ReadDir(dir)
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
	fi, err := c.sftp.Stat(p)
	if err != nil {
		return transfer.FileInfo{}, wrapErr(err)
	}
	return infoFromFS(fi), nil
}

func (c *Client) MakeDir(p string) error {
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
	fi, err := c.sftp.Lstat(p)
	if err != nil {
		return wrapErr(err)
	}
	if !fi.IsDir() || fi.Mode()&fs.ModeSymlink != 0 {
		return wrapErr(c.sftp.Remove(p))
	}
	entries, err := c.sftp.ReadDir(p)
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
	return wrapErr(c.sftp.RemoveDirectory(p))
}

func (c *Client) Rename(src, dst string) error {
	return wrapErr(c.sftp.Rename(src, dst))
}

// SupportsChmod is true because SFTP defines SETSTAT; an individual server may
// still refuse a given path, which surfaces as a normal error from Chmod.
func (c *Client) SupportsChmod() bool { return true }

func (c *Client) Chmod(p string, mode uint32) error {
	return wrapErr(c.sftp.Chmod(p, fs.FileMode(mode)))
}

func (c *Client) Download(p string) (io.ReadCloser, error) {
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
	return f, nil
}

func (c *Client) Upload(p string, r io.Reader) error {
	f, err := c.sftp.Create(p)
	if err != nil {
		return wrapErr(err)
	}
	// Close carries the server's status for the final flush, so discarding it
	// reports a quota or I/O failure at end-of-write as a successful upload.
	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("%w: %w", transfer.ErrTransferIncomplete, wrapErr(copyErr))
	}
	if closeErr != nil {
		return fmt.Errorf("%w: %w", transfer.ErrTransferIncomplete, wrapErr(closeErr))
	}
	return nil
}

func (c *Client) Ping() error {
	_, err := c.sftp.Getwd()
	return wrapErr(err)
}

func (c *Client) Close() error {
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

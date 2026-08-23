package ftp

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"strings"
	"time"

	jftp "github.com/jlaffaye/ftp"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// Client wraps jlaffaye/ftp and implements transfer.Client.
type Client struct {
	conn *jftp.ServerConn
}

// clampSize converts a server-reported uint64 size defensively - a hostile
// server could otherwise overflow it into a negative int64.
func clampSize(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// Dial connects and authenticates. passive controls passive/active mode. When
// tlsConfig is non-nil the control connection is upgraded with explicit TLS
// (AUTH TLS, RFC 4217) - i.e. FTPS.
func Dial(addr, user, pass string, passive bool, tlsConfig *tls.Config) (*Client, error) {
	opts := []jftp.DialOption{jftp.DialWithTimeout(10 * time.Second)}
	if tlsConfig != nil {
		opts = append(opts, jftp.DialWithExplicitTLS(tlsConfig))
	}
	conn, err := jftp.Dial(addr, opts...)
	if err != nil {
		if tlsConfig != nil && isTLSError(err) {
			return nil, fmt.Errorf("%w: %w", transfer.ErrTLSFailed, err)
		}
		return nil, fmt.Errorf("%w: %w", transfer.ErrConnectionFailed, err)
	}
	if err := conn.Login(user, pass); err != nil {
		_ = conn.Quit()
		return nil, fmt.Errorf("%w: %w", transfer.ErrAuthFailed, err)
	}
	return &Client{conn: conn}, nil
}

// isTLSError reports whether err is a TLS / certificate-verification failure, so
// the caller can surface ERR_TLS_FAILED (and hint at the insecure-skip-verify
// escape hatch for self-signed servers) instead of a generic connection error.
func isTLSError(err error) bool {
	var certErr *tls.CertificateVerificationError
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuth) || errors.As(err, &hostErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:") || strings.Contains(msg, "certificate")
}

func (c *Client) WorkingDir() (string, error) {
	return c.conn.CurrentDir()
}

func (c *Client) List(dir string) ([]transfer.FileInfo, error) {
	entries, err := c.conn.List(dir)
	if err != nil {
		return nil, err
	}
	out := make([]transfer.FileInfo, 0, len(entries))
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		out = append(out, transfer.FileInfo{
			Name:    e.Name,
			Size:    clampSize(e.Size),
			IsDir:   e.Type == jftp.EntryTypeFolder,
			ModTime: e.Time.Unix(),
		})
	}
	return out, nil
}

func (c *Client) Stat(p string) (transfer.FileInfo, error) {
	if p == "/" {
		return transfer.FileInfo{Name: "/", IsDir: true}, nil
	}
	parent := path.Dir(p)
	name := path.Base(p)
	entries, err := c.conn.List(parent)
	if err != nil {
		return transfer.FileInfo{}, err
	}
	for _, e := range entries {
		if e.Name == name {
			return transfer.FileInfo{
				Name:    e.Name,
				Size:    clampSize(e.Size),
				IsDir:   e.Type == jftp.EntryTypeFolder,
				ModTime: e.Time.Unix(),
			}, nil
		}
	}
	return transfer.FileInfo{}, fmt.Errorf("stat %s: not found", p)
}

func (c *Client) MakeDir(p string) error {
	return c.conn.MakeDir(p)
}

// Delete removes a file, falling back to a recursive directory removal only
// when the target actually is a directory. Retrying every DELE failure as
// RemoveDirRecur and returning that error instead discarded the real one, so a
// permission denial on a file surfaced as ERR_DIR_NOT_EMPTY and a dropped
// socket never reached isConnLost.
func (c *Client) Delete(p string) error {
	fi, statErr := c.Stat(p)
	if statErr == nil && fi.IsDir {
		return c.conn.RemoveDirRecur(p)
	}
	err := c.conn.Delete(p)
	if err != nil && statErr != nil {
		// Stat could not classify the target (some servers refuse LIST on a
		// directory), so the old fallback is still worth one attempt. The
		// original DELE error wins if it also fails.
		if rmErr := c.conn.RemoveDirRecur(p); rmErr == nil {
			return nil
		}
	}
	return err
}

func (c *Client) Rename(src, dst string) error {
	return c.conn.Rename(src, dst)
}

// SupportsChmod is false because Chmod below can only fail. Flip both together
// if SITE CHMOD ever lands, or the UI will offer a control that never works.
func (c *Client) SupportsChmod() bool { return false }

func (c *Client) Chmod(p string, mode uint32) error {
	// jlaffaye/ftp does not support SITE CHMOD, and FTP chmod is not
	// universally supported across servers anyway.
	return transfer.ErrPermissionsNotSupported
}

func (c *Client) Download(p string) (io.ReadCloser, error) {
	return c.conn.Retr(p)
}

func (c *Client) Upload(p string, r io.Reader) error {
	return c.conn.Stor(p, r)
}

func (c *Client) Ping() error {
	return c.conn.NoOp()
}

func (c *Client) Close() error {
	return c.conn.Quit()
}

// Ensure *Client implements transfer.Client at compile time.
var _ transfer.Client = (*Client)(nil)

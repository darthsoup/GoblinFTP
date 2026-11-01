package ftp

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/textproto"
	"path"
	"strings"
	"sync/atomic"
	"time"

	jftp "github.com/jlaffaye/ftp"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

const (
	dialTimeout       = 10 * time.Second
	controlTimeout    = 60 * time.Second
	dataTimeout       = 30 * time.Second
	completionTimeout = 10 * time.Second
)

// Client wraps jlaffaye/ftp and implements transfer.Client.
type Client struct {
	conn *jftp.ServerConn
}

// clampSize converts a server-reported uint64 size defensively: a hostile
// server could otherwise overflow it into a negative int64.
func clampSize(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// Dial connects and authenticates. passive controls passive/active mode; a
// non-nil tlsConfig upgrades the control connection to explicit TLS (FTPS, RFC 4217).
func Dial(addr, user, pass string, passive bool, tlsConfig *tls.Config) (*Client, error) {
	if tlsConfig != nil {
		tlsConfig = tlsConfig.Clone()
		tlsConfig.ClientSessionCache = tls.NewLRUClientSessionCache(4)
	}
	var control *deadlineConn
	opts := []jftp.DialOption{
		jftp.DialWithTimeout(dialTimeout),
		jftp.DialWithShutTimeout(completionTimeout),
		// jftp only bounds the TCP connect, so a server that accepts and then
		// stays silent blocks every later read forever and wedges the session.
		jftp.DialWithDialFunc(func(network, address string) (net.Conn, error) {
			conn, err := net.DialTimeout(network, address, dialTimeout)
			if err != nil {
				return nil, err
			}
			// jftp also uses this callback for data sockets, bypassing its TLS wrapper.
			if control != nil {
				wrapped := &deadlineConn{Conn: conn}
				wrapped.setTimeout(dataTimeout)
				if tlsConfig != nil {
					return tls.Client(wrapped, tlsConfig), nil
				}
				return wrapped, nil
			}
			// Tight during the handshake so a server that accepts and never
			// sends its greeting fails fast, relaxed afterwards for real work.
			wrapped := &deadlineConn{Conn: conn}
			wrapped.setTimeout(dialTimeout)
			control = wrapped
			return wrapped, nil
		}),
	}
	if !passive {
		// jlaffaye has no PORT/EPRT support at all, so "active" can only mean
		// falling back from EPSV to PASV.
		opts = append(opts, jftp.DialWithDisabledEPSV(true))
	}
	if tlsConfig != nil {
		opts = append(opts, jftp.DialWithExplicitTLS(tlsConfig))
	}
	conn, err := jftp.Dial(addr, opts...)
	if err != nil {
		switch {
		case tlsConfig != nil && isTLSError(err):
			return nil, fmt.Errorf("%w: %w", transfer.ErrTLSFailed, err)
		case isTimeout(err):
			return nil, fmt.Errorf("%w: %w", transfer.ErrConnectionTimeout, err)
		default:
			return nil, fmt.Errorf("%w: %w", transfer.ErrConnectionFailed, err)
		}
	}
	if err := conn.Login(user, pass); err != nil {
		_ = conn.Quit()
		return nil, classifyLogin(err, tlsConfig != nil)
	}
	if control != nil {
		control.setTimeout(controlTimeout)
	}
	return &Client{conn: conn}, nil
}

// classifyLogin separates a real credential rejection from everything else
// Login can fail on. The explicit-TLS handshake is deferred to the first write,
// which is USER, so a bad certificate surfaces here and not from Dial.
func classifyLogin(err error, tlsEnabled bool) error {
	if tlsEnabled && isTLSError(err) {
		return fmt.Errorf("%w: %w", transfer.ErrTLSFailed, err)
	}
	if isTimeout(err) {
		return fmt.Errorf("%w: %w", transfer.ErrConnectionTimeout, err)
	}
	var te *textproto.Error
	if errors.As(err, &te) {
		switch te.Code {
		case replyNotLoggedIn, replyNeedAccount, 430:
			return fmt.Errorf("%w: %w", transfer.ErrAuthFailed, err)
		case replyServiceNotAvailable:
			return fmt.Errorf("%w: %w", transfer.ErrConnectionFailed, err)
		}
		return fmt.Errorf("%w: %w", transfer.ErrConnectionFailed, err)
	}
	// jftp issues USER with cmd(-1, ...), so textproto never builds an error
	// for it and the reply code is gone by the time it reaches us.
	if isAuthText(err.Error()) {
		return fmt.Errorf("%w: %w", transfer.ErrAuthFailed, err)
	}
	return fmt.Errorf("%w: %w", transfer.ErrConnectionFailed, err)
}

func isAuthText(msg string) bool {
	m := strings.ToLower(msg)
	return contains(m, "not logged in", "login incorrect", "authentication failed",
		"login or password incorrect", "invalid user")
}

// Explicit completion deadlines take precedence over rolling inactivity limits.
type deadlineConn struct {
	net.Conn
	timeout  atomic.Int64
	explicit atomic.Int64
}

func (d *deadlineConn) setTimeout(t time.Duration) { d.timeout.Store(int64(t)) }

func (d *deadlineConn) deadline() time.Time {
	deadline := time.Now().Add(time.Duration(d.timeout.Load()))
	if n := d.explicit.Load(); n != 0 && time.Unix(0, n).Before(deadline) {
		return time.Unix(0, n)
	}
	return deadline
}

func (d *deadlineConn) SetDeadline(t time.Time) error {
	n := int64(0)
	if !t.IsZero() {
		n = t.UnixNano()
	}
	d.explicit.Store(n)
	return d.Conn.SetDeadline(t)
}

func (d *deadlineConn) Read(b []byte) (int, error) {
	if err := d.SetReadDeadline(d.deadline()); err != nil {
		return 0, err
	}
	n, err := d.Conn.Read(b)
	if isTimeout(err) {
		_ = d.Close()
	}
	return n, err
}

func (d *deadlineConn) Write(b []byte) (int, error) {
	if err := d.SetWriteDeadline(d.deadline()); err != nil {
		return 0, err
	}
	n, err := d.Conn.Write(b)
	if isTimeout(err) {
		_ = d.Close()
	}
	return n, err
}

// isTLSError reports whether err is a TLS or certificate-verification failure, so
// the caller can surface ERR_TLS_FAILED instead of a generic connection error.
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
	entries, err := c.conn.ListWithLimits(dir, 100000, 16<<20)
	if err != nil {
		return nil, wrapErr(err)
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
			ModTime: modTime(e.Time),
		})
	}
	return out, nil
}

func (c *Client) Stat(p string) (transfer.FileInfo, error) {
	if transfer.ComparisonPath(p, true) == "/" {
		return transfer.FileInfo{Name: "/", IsDir: true}, nil
	}
	if entry, err := c.conn.GetEntry(p); err == nil {
		return transfer.FileInfo{Name: entry.Name, Size: clampSize(entry.Size), IsDir: entry.Type == jftp.EntryTypeFolder, ModTime: modTime(entry.Time)}, nil
	} else {
		var reply *textproto.Error
		if !errors.As(err, &reply) || (reply.Code != 500 && reply.Code != 502 && reply.Code != 504) {
			return transfer.FileInfo{}, wrapErr(err)
		}
	}

	candidates := []string{p}
	if normalized := strings.ReplaceAll(p, `\`, "/"); normalized != p {
		candidates = append(candidates, normalized)
	}
	for _, candidate := range candidates {
		entries, err := c.conn.ListWithLimits(path.Dir(candidate), 100000, 16<<20)
		if err != nil {
			wrapped := wrapErr(err)
			if errors.Is(wrapped, transfer.ErrNotFound) {
				continue
			}
			return transfer.FileInfo{}, wrapped
		}
		var found *jftp.Entry
		for _, e := range entries {
			if e.Name == path.Base(candidate) {
				return transfer.FileInfo{Name: e.Name, Size: clampSize(e.Size), IsDir: e.Type == jftp.EntryTypeFolder, ModTime: modTime(e.Time)}, nil
			}
			if strings.EqualFold(e.Name, path.Base(candidate)) {
				if found != nil {
					return transfer.FileInfo{}, transfer.ErrFileExists
				}
				found = e
			}
		}
		if found != nil {
			return transfer.FileInfo{Name: found.Name, Size: clampSize(found.Size), IsDir: found.Type == jftp.EntryTypeFolder, ModTime: modTime(found.Time)}, nil
		}
	}

	return transfer.FileInfo{}, transfer.ErrNotFound
}

func (c *Client) MakeDir(p string) error {
	return wrapErr(c.conn.MakeDir(p))
}

func (c *Client) Delete(p string) error {
	if _, err := transfer.CleanDeletePath(p); err != nil {
		return err
	}
	budget := 100000
	return c.deleteDepth(p, 0, &budget)
}

func (c *Client) deleteDepth(p string, depth int, budget *int) error {
	if depth > 64 || *budget <= 0 {
		return fmt.Errorf("%w: remote tree exceeds limit", transfer.ErrInvalidType)
	}
	*budget--
	fi, err := c.Stat(p)
	if err != nil {
		return err
	}
	if !fi.IsDir {
		return wrapErr(c.conn.Delete(p))
	}
	entries, err := c.conn.ListWithLimits(p, 100000, 16<<20)
	if err != nil {
		return wrapErr(err)
	}
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		if err := transfer.ValidateChildName(e.Name, true); err != nil {
			return err
		}
	}
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		child := strings.TrimRight(p, "/") + "/" + e.Name
		if e.Type == jftp.EntryTypeFolder {
			err = c.deleteDepth(child, depth+1, budget)
		} else {
			*budget--
			if *budget < 0 {
				return transfer.ErrInvalidType
			}
			err = wrapErr(c.conn.Delete(child))
		}
		if err != nil {
			return err
		}
	}
	return wrapErr(c.conn.RemoveDir(p))
}

func (c *Client) Rename(src, dst string) error {
	return wrapErr(c.conn.Rename(src, dst))
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
	resp, err := c.conn.Retr(p)
	if err != nil {
		return nil, wrapErr(err)
	}
	return resp, nil
}

// Upload reports a mid-transfer failure as ErrTransferIncomplete: jftp joins the
// copy, data-close, and final-status errors, and any of them leaves a partial
// file on the server rather than nothing at all.
func (c *Client) Upload(p string, r io.Reader) error {
	err := c.conn.Stor(p, r)
	if err == nil {
		return nil
	}
	wrapped := wrapErr(err)
	if errors.Is(wrapped, transfer.ErrQuotaExceeded) || errors.Is(wrapped, transfer.ErrPermissionDenied) {
		return wrapped
	}
	return fmt.Errorf("%w: %w", transfer.ErrTransferIncomplete, wrapped)
}

func (c *Client) Ping() error {
	return wrapErr(c.conn.NoOp())
}

func (c *Client) Close() error {
	return c.conn.Quit()
}

var _ transfer.Client = (*Client)(nil)

// modTime returns 0 for an MLSD entry with no modify fact, so the API can emit an
// empty timestamp instead of rendering year 1.
func modTime(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

package api

import (
	"archive/zip"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/metrics"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// sanitizeFilename removes characters that could break HTTP header values.
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r == '"' || r == '\\' || r == '\r' || r == '\n' || r == '\t' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IssueDownloadToken issues a signed short-lived token for downloading a file.
func (h *Handler) IssueDownloadToken(c echo.Context) error {
	sess, ok := c.Get("session").(*auth.Session)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active session"))
	}
	var req struct {
		Path string `json:"path"`
	}
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if req.Path == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "path is required"))
	}
	expiry := time.Now().Add(15 * time.Minute)
	// sess.DownloadKey, never sess.ID: the token is only base64, so a session ID
	// here could be replayed as a gftp_session cookie for full account access.
	tok := transfer.IssueToken(h.cfg.DownloadTokenSecret, sess.DownloadKey, req.Path, expiry)
	return OK(c, map[string]string{"token": tok})
}

// DownloadFile is a public endpoint that streams a file using a signed token.
func (h *Handler) DownloadFile(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrInvalidToken, "token is required"))
	}
	downloadKey, filePath, err := transfer.ValidateToken(h.cfg.DownloadTokenSecret, token)
	if err != nil {
		// The message is the caller's own token being rejected, so it must not
		// carry the internal error text (see classify() in errclass.go).
		return Fail(c, gftperrors.New(gftperrors.ErrInvalidToken, "download link is invalid or has expired"))
	}
	sess, ok := h.store.GetByDownloadKey(downloadKey)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "session not found"))
	}
	clientVal, ok := sess.Get("client")
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	client, ok := clientVal.(transfer.Client)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	// Serialize against other transfers on this session's single control
	// connection for the whole streamed download.
	sess.LockTransfer()
	defer sess.UnlockTransfer()
	// A failed open is not evidence of absence: it can equally be a denial, a
	// directory, or a dead data connection, so let classify decide.
	r, err := client.Download(filePath)
	if err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = r.Close()
		}
	}()
	src := metrics.CountingReader(r, h.metrics.TransferBytes.WithLabelValues("download", protocolFromSession(sess)))

	filename := sanitizeFilename(path.Base(filePath))
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	// With a Content-Length the browser rejects a short body by itself. Without
	// one a truncated chunked stream still terminates cleanly and saves as whole.
	lengthKnown := false
	if fi, statErr := client.Stat(filePath); statErr == nil && fi.Size > 0 && !fi.IsDir {
		c.Response().Header().Set("Content-Length", strconv.FormatInt(fi.Size, 10))
		lengthKnown = true
	}
	c.Response().WriteHeader(http.StatusOK)

	// The transfer lock is held for the whole stream, so a client reading at a
	// trickle would block the session. The deadline is per write, not per transfer.
	_, copyErr := io.Copy(&deadlineWriter{c: c, w: c.Response()}, src)
	// Close carries the FTP "226 transfer complete" status, the only place a
	// truncated RETR is observable, so it cannot be deferred away.
	closeErr := r.Close()
	closed = true

	streamErr := errors.Join(copyErr, closeErr)
	if streamErr == nil {
		return nil
	}
	// The status is already committed, so this is the only way the access line
	// and Sentry learn the transfer failed.
	code, _ := classify(streamErr)
	c.Set(LoggedErrorKey, gftperrors.New(code, "download aborted mid-stream").WithCause(streamErr))
	if !lengthKnown {
		// Break the connection instead of ending the chunked stream cleanly.
		// echo's Recover re-panics this, and net/http drops the connection.
		panic(http.ErrAbortHandler)
	}
	return nil
}

// downloadWriteTimeout is how long a single write may block before the client
// is treated as stalled.
const downloadWriteTimeout = 60 * time.Second

// deadlineWriter refreshes the connection's write deadline before every write.
type deadlineWriter struct {
	c echo.Context
	w io.Writer
}

func (d *deadlineWriter) Write(p []byte) (int, error) {
	// Best-effort: a server without deadline support (or a test recorder) just
	// keeps the previous behavior rather than failing the download.
	_ = http.NewResponseController(d.c.Response()).SetWriteDeadline(time.Now().Add(downloadWriteTimeout))
	return d.w.Write(p)
}

// DownloadZip assembles multiple remote paths into a ZIP and sends it to the browser.
// Built fully before any header, so a failure mid-archive is still reportable.
func (h *Handler) DownloadZip(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	var req struct {
		Paths []string `json:"paths"`
	}
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if len(req.Paths) == 0 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "paths are required"))
	}
	var totalSize int64
	for _, p := range req.Paths {
		size, err := zipInputSize(client, p)
		if err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		totalSize += size
		if totalSize > maxZipSize {
			return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "archive exceeds maximum size"))
		}
	}
	sess, _ := c.Get("session").(*auth.Session)
	counter := h.metrics.TransferBytes.WithLabelValues("download", protocolFromSession(sess))

	// Spooled to disk, not a bytes.Buffer: one buffer per concurrent request held
	// the whole archive (up to maxZipSize), which a few users turned into an OOM.
	tmp, err := os.CreateTemp(h.cfg.DataDir, "gftp-zip-*")
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "could not stage archive").WithCause(err))
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	zw := zip.NewWriter(tmp)
	for _, p := range req.Paths {
		if err := addToZip(zw, client, p, "", counter); err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
	}
	if err := zw.Close(); err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "failed to finalize archive").WithCause(err))
	}
	size, err := tmp.Seek(0, io.SeekEnd)
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "failed to stage archive").WithCause(err))
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "failed to stage archive").WithCause(err))
	}

	c.Response().Header().Set("Content-Disposition", `attachment; filename="archive.zip"`)
	c.Response().Header().Set("Content-Type", "application/zip")
	c.Response().Header().Set("Content-Length", strconv.FormatInt(size, 10))
	c.Response().WriteHeader(http.StatusOK)
	_, copyErr := io.Copy(&deadlineWriter{c: c, w: c.Response()}, tmp)
	return copyErr
}

func zipInputSize(client transfer.Client, remotePath string) (int64, error) {
	return zipInputSizeDepth(client, remotePath, 0)
}

func zipInputSizeDepth(client transfer.Client, remotePath string, depth int) (int64, error) {
	// The maxZipSize guard below counts file bytes only, so a loop of empty
	// directories accumulates nothing and would recurse until the stack blows.
	if depth > maxTreeDepth {
		return 0, errTreeTooDeep
	}
	fi, err := client.Stat(remotePath)
	if err != nil {
		return 0, err
	}
	if !fi.IsDir {
		return fi.Size, nil
	}
	entries, err := client.List(remotePath)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		size, err := zipInputSizeDepth(client, path.Join(remotePath, entry.Name), depth+1)
		if err != nil {
			return 0, err
		}
		total += size
		if total > maxZipSize {
			return total, nil
		}
	}
	return total, nil
}

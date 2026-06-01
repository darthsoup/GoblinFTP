// backend/internal/api/download.go
package api

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
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
	if err := c.Bind(&req); err != nil || req.Path == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "path is required"))
	}
	expiry := time.Now().Add(15 * time.Minute)
	tok, err := transfer.IssueToken(h.cfg.DownloadTokenSecret, sess.ID, req.Path, expiry)
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "failed to issue token"))
	}
	return OK(c, map[string]string{"token": tok})
}

// DownloadFile is a public endpoint that streams a file using a signed token.
func (h *Handler) DownloadFile(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrInvalidToken, "token is required"))
	}
	sessionID, filePath, err := transfer.ValidateToken(h.cfg.DownloadTokenSecret, token)
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInvalidToken, err.Error()))
	}
	sess, ok := h.store.Get(sessionID)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "session not found"))
	}
	client, ok := sess.Data["client"].(transfer.Client)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	r, err := client.Download(filePath)
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrFileNotFound, err.Error()))
	}
	defer r.Close()

	filename := sanitizeFilename(path.Base(filePath))
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().WriteHeader(http.StatusOK)
	_, copyErr := io.Copy(c.Response(), r)
	return copyErr
}

// DownloadZip assembles multiple remote paths into a ZIP and sends it to the browser.
// The archive is built in memory before writing to avoid sending 200 OK and then
// being unable to report errors partway through the stream.
// Reuses addToZip from archive.go (same package).
func (h *Handler) DownloadZip(c echo.Context) error {
	client, ok := clientFromContext(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := c.Bind(&req); err != nil || len(req.Paths) == 0 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "paths are required"))
	}
	var totalSize int64
	for _, p := range req.Paths {
		size, err := zipInputSize(client, p)
		if err != nil {
			return Fail(c, gftperrors.New(gftperrors.ErrOperationFailed, err.Error()))
		}
		totalSize += size
		if totalSize > maxZipSize {
			return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "archive exceeds maximum size"))
		}
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range req.Paths {
		if err := addToZip(zw, client, p, ""); err != nil {
			return Fail(c, gftperrors.New(gftperrors.ErrOperationFailed, err.Error()))
		}
	}
	if err := zw.Close(); err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "failed to finalise archive"))
	}
	c.Response().Header().Set("Content-Disposition", `attachment; filename="archive.zip"`)
	c.Response().Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	return c.Blob(http.StatusOK, "application/zip", buf.Bytes())
}

func zipInputSize(client transfer.Client, remotePath string) (int64, error) {
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
		size, err := zipInputSize(client, path.Join(remotePath, entry.Name))
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

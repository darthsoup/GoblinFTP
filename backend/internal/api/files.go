// backend/internal/api/files.go
package api

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/logging"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// fileInfoJSON is the API wire representation of a remote filesystem entry.
type fileInfoJSON struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	IsDir    bool   `json:"isDir"`
	Modified string `json:"modified"` // RFC3339
	Mode     string `json:"mode"`     // e.g. "drwxr-xr-x"
}

func toFileInfoJSON(fi transfer.FileInfo) fileInfoJSON {
	return fileInfoJSON{
		Name:     fi.Name,
		Size:     fi.Size,
		IsDir:    fi.IsDir,
		Modified: time.Unix(fi.ModTime, 0).UTC().Format(time.RFC3339),
		Mode:     fi.Permissions,
	}
}

// clientFromContext extracts the transfer.Client and its session from the
// session stored by requireSession middleware.
func clientFromContext(c echo.Context) (transfer.Client, *auth.Session, bool) {
	sess, ok := c.Get("session").(*auth.Session)
	if !ok {
		return nil, nil, false
	}
	v, ok := sess.Get("client")
	if !ok {
		return nil, sess, false
	}
	client, ok := v.(transfer.Client)
	return client, sess, ok
}

// lockedClient returns the session's transfer client with the per-session
// transfer lock HELD, plus a release func the caller MUST defer. Only one client
// operation runs at a time per session, so concurrent requests never interleave
// two data transfers on the single FTP/SFTP control connection (jlaffaye/ftp's
// ServerConn is not safe for concurrent use). Returns (nil, nil, false) when
// there is no active connection, in which case the lock is not taken.
func lockedClient(c echo.Context) (transfer.Client, func(), bool) {
	client, sess, ok := clientFromContext(c)
	if !ok {
		return nil, nil, false
	}
	sess.LockTransfer()
	return client, sess.UnlockTransfer, true
}

func (h *Handler) ListFiles(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	path := c.QueryParam("path")
	if path == "" {
		path = "/"
	}
	files, err := client.List(path)
	if err != nil {
		return failClient(c, gftperrors.ErrListFailed, err)
	}
	result := make([]fileInfoJSON, len(files))
	for i, f := range files {
		result[i] = toFileInfoJSON(f)
	}
	return OK(c, result)
}

func (h *Handler) CreateDirectory(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	var req struct {
		Path string `json:"path"`
	}
	if err := c.Bind(&req); err != nil || req.Path == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "path is required"))
	}
	if err := ensureDirAll(client, req.Path); err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	return OK(c, nil)
}

type deleteResult struct {
	Deleted []string       `json:"deleted"`
	Failed  []deleteFailed `json:"failed"`
}

type deleteFailed struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (h *Handler) DeleteFiles(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := c.Bind(&req); err != nil || len(req.Paths) == 0 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "paths are required"))
	}
	result := deleteResult{}
	for _, p := range req.Paths {
		err := client.Delete(p)
		if err == nil {
			result.Deleted = append(result.Deleted, p)
			continue
		}
		// A dropped connection aborts the whole batch and triggers the SPA's
		// reconnect flow, instead of reporting every remaining path as failed.
		if isConnLost(err) {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		// Per-item failures are part of a successful (HTTP 200) batch response;
		// classify into a stable code + friendly message and log the raw cause
		// here (it never reaches the client and no longer flows through Fail()).
		code, msg := classify(err)
		result.Failed = append(result.Failed, deleteFailed{Path: p, Code: string(code), Message: msg})
		attrs := []slog.Attr{slog.String("path", p), slog.String("code", string(code))}
		attrs = append(attrs, logging.SafeLogAttrs(slog.String("cause", err.Error()))...)
		h.logger.LogAttrs(c.Request().Context(), slog.LevelWarn, "delete failed", attrs...)
	}
	// Always a 200 success once the request was processed; per-item outcomes live
	// in data so the SPA surfaces which items failed and why.
	return OK(c, result)
}

func (h *Handler) RenameFile(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.Bind(&req); err != nil || req.From == "" || req.To == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "from and to are required"))
	}
	if err := client.Rename(req.From, req.To); err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	return OK(c, nil)
}

func (h *Handler) CopyFile(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.Bind(&req); err != nil || req.From == "" || req.To == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "from and to are required"))
	}
	if err := copyTree(client, req.From, req.To); err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	return OK(c, nil)
}

// ensureDirAll creates dir and any missing parents, idempotently and uniformly
// across FTP and SFTP. FTP's MakeDir is single-level and errors if the target
// already exists, while SFTP's is recursive; the Stat guard normalizes both so a
// folder upload can recreate a tree without spurious "already exists" failures.
func ensureDirAll(client transfer.Client, dir string) error {
	dir = path.Clean(dir)
	if dir == "/" || dir == "." || dir == "" {
		return nil
	}
	if fi, err := client.Stat(dir); err == nil {
		if fi.IsDir {
			return nil
		}
		return errors.New("destination parent exists and is not a directory")
	}
	if parent := path.Dir(dir); parent != dir {
		if err := ensureDirAll(client, parent); err != nil {
			return err
		}
	}
	if err := client.MakeDir(dir); err != nil {
		// Tolerate an idempotent/raced create (SFTP MkdirAll, or another request
		// won the race): an existing directory is success.
		if fi, statErr := client.Stat(dir); statErr == nil && fi.IsDir {
			return nil
		}
		return err
	}
	return nil
}

// copyTree copies from src to dst, recursing into directories. Files are streamed
// via Download→Upload (Upload overwrites). For directories it recreates the tree,
// only calling MakeDir when dst doesn't already exist so an overwrite merges into
// the existing directory rather than failing.
func copyTree(client transfer.Client, src, dst string) error {
	info, err := client.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir {
		if _, err := client.Stat(dst); err != nil {
			if err := client.MakeDir(dst); err != nil {
				return err
			}
		}
		entries, err := client.List(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTree(client, path.Join(src, e.Name), path.Join(dst, e.Name)); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFile(client, src, dst)
}

// copyFile copies a single file. The download is staged to a temp file and fully
// closed before the upload starts: FTP allows only one data transfer per control
// connection at a time, so streaming Download→Upload directly would interleave
// RETR and STOR and desync the control channel.
func copyFile(client transfer.Client, src, dst string) error {
	r, err := client.Download(src)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "gftp-copy-*")
	if err != nil {
		_ = r.Close()
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	_, copyErr := io.Copy(tmp, r)
	_ = r.Close() // completes RETR before the upload's STOR
	if copyErr != nil {
		return copyErr
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return client.Upload(dst, tmp)
}

func (h *Handler) SetPermissions(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	var req struct {
		Path string  `json:"path"`
		Mode *uint32 `json:"mode"`
	}
	if err := c.Bind(&req); err != nil || req.Path == "" || req.Mode == nil {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "path and mode are required"))
	}
	if err := client.Chmod(req.Path, *req.Mode); err != nil {
		if errors.Is(err, transfer.ErrPermissionsNotSupported) {
			return Fail(c, gftperrors.New(gftperrors.ErrPermissionsNotSupported, "chmod not supported by server"))
		}
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	return OK(c, nil)
}

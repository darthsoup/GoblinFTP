package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strings"
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

// lockedClient returns the session's transfer client with the per-session lock
// HELD plus a release func the caller MUST defer: ServerConn is not concurrency-safe.
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
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if req.Path == "" {
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
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if len(req.Paths) == 0 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "paths are required"))
	}
	// "/" reaches RemoveDirRecur and wipes the whole account. Refuse the batch
	// outright rather than reporting it afterwards as one failed path.
	for _, p := range req.Paths {
		if isRootPath(p) {
			return Fail(c, gftperrors.New(gftperrors.ErrBadRequest,
				"the remote root cannot be deleted"))
		}
	}
	// Initialize non-nil so the JSON always carries arrays, never null: the SPA
	// reads result.failed.length unconditionally and would throw.
	result := deleteResult{Deleted: []string{}, Failed: []deleteFailed{}}
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
		// Per-item failures ride in a successful (HTTP 200) batch response, so the
		// raw cause is logged here rather than through Fail().
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
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if req.From == "" || req.To == "" {
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
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if req.From == "" || req.To == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "from and to are required"))
	}
	if err := copyTree(client, h.cfg.DataDir, req.From, req.To); err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	return OK(c, nil)
}

// ensureDirAll creates dir and any missing parents, idempotently across FTP and
// SFTP: FTP's MakeDir is single-level and errors when the target already exists.
func ensureDirAll(client transfer.Client, dir string) error {
	_, err := ensureDirAllCreated(client, dir)
	return err
}

// ensureDirAllCreated is ensureDirAll, also reporting whether it created anything.
// A fresh parent cannot hold the destination, so callers skip their probe (a LIST).
func ensureDirAllCreated(client transfer.Client, dir string) (bool, error) {
	dir = path.Clean(dir)
	if dir == "/" || dir == "." || dir == "" {
		return false, nil
	}
	if fi, err := client.Stat(dir); err == nil {
		if fi.IsDir {
			return false, nil
		}
		return false, errors.New("destination parent exists and is not a directory")
	}
	if parent := path.Dir(dir); parent != dir {
		if _, err := ensureDirAllCreated(client, parent); err != nil {
			return false, err
		}
	}
	if err := client.MakeDir(dir); err != nil {
		// Tolerate an idempotent or raced create: an existing directory is success,
		// but we did not create it, so the caller must still probe.
		if fi, statErr := client.Stat(dir); statErr == nil && fi.IsDir {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// copyTree copies src to dst recursively. MakeDir runs only when dst is missing,
// so an overwrite merges into the existing directory rather than failing.
func copyTree(client transfer.Client, dataDir, src, dst string) error {
	return copyTreeDepth(client, dataDir, src, dst, 0)
}

func copyTreeDepth(client transfer.Client, dataDir, src, dst string, depth int) error {
	if depth > maxTreeDepth {
		return errTreeTooDeep
	}
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
			if err := copyTreeDepth(client, dataDir, path.Join(src, e.Name), path.Join(dst, e.Name), depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFile(client, dataDir, src, dst)
}

// copyFile stages the download to a temp file and closes it before uploading:
// FTP allows one data transfer per control connection, so streaming desyncs it.
func copyFile(client transfer.Client, dataDir, src, dst string) error {
	r, err := client.Download(src)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dataDir, "gftp-copy-*")
	if err != nil {
		_ = r.Close()
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	_, copyErr := io.Copy(tmp, r)
	// Close completes RETR before the upload's STOR and carries the server's
	// final status: discarding it uploads a truncated read as a full copy.
	closeErr := r.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return fmt.Errorf("%w: %w", transfer.ErrTransferIncomplete, err)
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
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if req.Path == "" || req.Mode == nil {
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

// isRootPath guards destructive operations against the remote root. The adapters
// refuse it too; this stops the request before it reaches them.
func isRootPath(p string) bool {
	cleaned := path.Clean(strings.TrimSpace(p))
	return cleaned == "/" || cleaned == "." || cleaned == ""
}

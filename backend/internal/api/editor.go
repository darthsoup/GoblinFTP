package api

import (
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

const maxEditorReadSize int64 = 1 * 1024 * 1024 // 1 MB

// isEditableExtension returns true when the given extension (without dot) is in
// the allowed list and the editor is not globally disabled.
func (h *Handler) isEditableExtension(ext string) bool {
	if h.cfg.Settings.Editor.Disabled {
		return false
	}
	for _, allowed := range h.cfg.Settings.Editor.AllowedExtensions {
		if strings.EqualFold(allowed, ext) {
			return true
		}
	}
	return false
}

type writeFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	// ExpectedVersion is an If-Match-style precondition carrying the version the
	// client saw when it opened the file. nil means the client made no claim,
	// which is rejected rather than silently written.
	ExpectedVersion *string `json:"expectedVersion"`
	// Overwrite skips the precondition entirely — the user was shown the
	// conflict and chose to replace the server's copy.
	Overwrite bool `json:"overwrite"`
}

// fileVersion is the editor's optimistic-concurrency token. It is opaque to
// clients on purpose: they must not parse or reconstruct it, so the precision
// policy (and any later move to a content hash or FTP MDTM) stays server-side.
func fileVersion(fi transfer.FileInfo) string {
	return strconv.FormatInt(fi.Size, 10) + ":" + strconv.FormatInt(fi.ModTime, 10)
}

// fileMeta is the version block returned by both read and write. Version is nil
// when the server could not stat the path, which tells the client it has no
// conflict detection for this file and must save unconditionally.
type fileMeta struct {
	Version  *string `json:"version"`
	Size     int64   `json:"size"`
	Modified string  `json:"modified"`
}

func metaFrom(fi transfer.FileInfo, found bool) fileMeta {
	if !found {
		return fileMeta{}
	}
	v := fileVersion(fi)
	return fileMeta{
		Version:  &v,
		Size:     fi.Size,
		Modified: time.Unix(fi.ModTime, 0).UTC().Format(time.RFC3339),
	}
}

type readFileResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	fileMeta
}

type writeFileResult struct {
	fileMeta
}

// statForVersion mirrors destinationTaken's semantics (an ordinary Stat failure
// means "absent", a dead connection is surfaced) but keeps the FileInfo.
// Callers must already hold the per-session transfer lock.
func statForVersion(client transfer.Client, p string) (transfer.FileInfo, bool, error) {
	fi, err := client.Stat(p)
	if err == nil {
		return fi, true, nil
	}
	if isConnLost(err) {
		return transfer.FileInfo{}, false, err
	}
	return transfer.FileInfo{}, false, nil
}

// ReadFile handles GET /api/files/read?path=<remote-path>
// Returns the content plus a version token for text files up to 1 MB.
func (h *Handler) ReadFile(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "no active connection"))
	}
	defer release()

	path := c.QueryParam("path")
	if path == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "path is required"))
	}

	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	if !h.isEditableExtension(ext) {
		return Fail(c, gftperrors.New(gftperrors.ErrEditorDisabled, "file type not editable"))
	}

	// Stat before Download, not after. A write landing between the two would
	// otherwise hand the client a token matching content it never saw, and the
	// next save would destroy that write silently. This order yields a token
	// older than the content instead, i.e. a spurious conflict — the safe way to
	// lose the race.
	fi, found, statErr := statForVersion(client, path)
	if statErr != nil {
		return failClient(c, gftperrors.ErrOperationFailed, statErr)
	}

	r, err := client.Download(path)
	if err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	defer r.Close()

	lr := &io.LimitedReader{R: r, N: maxEditorReadSize + 1}
	data, err := io.ReadAll(lr)
	if err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	if int64(len(data)) > maxEditorReadSize {
		return Fail(c, gftperrors.New(gftperrors.ErrFileTooLarge, "file exceeds 1 MB editor limit"))
	}

	return OK(c, readFileResult{
		Path:     path,
		Content:  string(data),
		fileMeta: metaFrom(fi, found),
	})
}

// WriteFile handles POST /api/files/write.
func (h *Handler) WriteFile(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "no active connection"))
	}
	defer release()

	if h.cfg.Settings.Editor.ViewOnly {
		return Fail(c, gftperrors.New(gftperrors.ErrEditorDisabled, "editor is in view-only mode"))
	}

	var req writeFileRequest
	if err := c.Bind(&req); err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "invalid request body"))
	}
	if req.Path == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "path is required"))
	}

	ext := strings.TrimPrefix(filepath.Ext(req.Path), ".")
	if !h.isEditableExtension(ext) {
		return Fail(c, gftperrors.New(gftperrors.ErrEditorDisabled, "file type not editable"))
	}
	if int64(len(req.Content)) > maxEditorReadSize {
		return Fail(c, gftperrors.New(gftperrors.ErrFileTooLarge, "content exceeds 1 MB editor limit"))
	}

	// Neither protocol offers compare-and-swap, so the gap between this check and
	// the Upload below is inherently racy. It narrows the window from the whole
	// editing session to one round trip; it does not close it.
	switch {
	case req.Overwrite:
		// The user was shown the conflict and chose to replace.
	case req.ExpectedVersion == nil:
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "expectedVersion or overwrite is required"))
	default:
		fi, found, statErr := statForVersion(client, req.Path)
		switch {
		case statErr != nil:
			return failClient(c, gftperrors.ErrOperationFailed, statErr)
		case !found:
			return Fail(c, gftperrors.New(gftperrors.ErrFileNotFound, "the file no longer exists on the server"))
		case fileVersion(fi) != *req.ExpectedVersion:
			return Fail(c, gftperrors.New(gftperrors.ErrFileModified,
				"the file changed on the server since it was opened"))
		}
	}

	if err := client.Upload(req.Path, strings.NewReader(req.Content)); err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}

	// Re-stat so the client adopts the new baseline. Without this its very next
	// save would conflict with the write it just made.
	fi, found, statErr := statForVersion(client, req.Path)
	if statErr != nil {
		return failClient(c, gftperrors.ErrOperationFailed, statErr)
	}

	return OK(c, writeFileResult{fileMeta: metaFrom(fi, found)})
}

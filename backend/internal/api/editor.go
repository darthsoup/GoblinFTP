package api

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
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
}

// ReadFile handles GET /api/files/read?path=<remote-path>
// Returns { content: string, path: string } for text files up to 1 MB.
func (h *Handler) ReadFile(c echo.Context) error {
	client, ok := clientFromContext(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "no active connection"))
	}

	path := c.QueryParam("path")
	if path == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "path is required"))
	}

	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	if !h.isEditableExtension(ext) {
		return Fail(c, gftperrors.New(gftperrors.ErrEditorDisabled, "file type not editable"))
	}

	r, err := client.Download(path)
	if err != nil {
		return Fail(c, gftperrors.Wrap(gftperrors.ErrOperationFailed, err))
	}
	defer r.Close()

	lr := &io.LimitedReader{R: r, N: maxEditorReadSize + 1}
	data, err := io.ReadAll(lr)
	if err != nil {
		return Fail(c, gftperrors.Wrap(gftperrors.ErrOperationFailed, err))
	}
	if int64(len(data)) > maxEditorReadSize {
		return Fail(c, gftperrors.New(gftperrors.ErrFileTooLarge, "file exceeds 1 MB editor limit"))
	}

	return OK(c, map[string]string{
		"content": string(data),
		"path":    path,
	})
}

// WriteFile handles POST /api/files/write.
func (h *Handler) WriteFile(c echo.Context) error {
	client, ok := clientFromContext(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "no active connection"))
	}

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

	if err := client.Upload(req.Path, strings.NewReader(req.Content)); err != nil {
		return Fail(c, gftperrors.Wrap(gftperrors.ErrOperationFailed, err))
	}

	return OK(c, nil)
}

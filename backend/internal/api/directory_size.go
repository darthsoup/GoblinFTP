package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

func (h *Handler) DirectorySize(c echo.Context) error {
	client, sess, ok := clientFromContext(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	var req struct {
		Path string `json:"path"`
	}
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if !transfer.ValidateDirectorySizePath(req.Path) {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "an absolute directory path is required"))
	}
	if !sess.BeginDirectoryScan() {
		return Fail(c, gftperrors.New(gftperrors.ErrOperationInProgress, "a directory size calculation is already running"))
	}
	defer sess.EndDirectoryScan()
	reader := &sessionDirectoryReader{client: client, handler: h, session: sess}
	result, err := transfer.CalculateDirectorySize(c.Request().Context(), reader, req.Path)
	if err != nil {
		if c.Request().Context().Err() != nil {
			return nil
		}
		var gerr *gftperrors.GFTPError
		if errors.As(err, &gerr) {
			return Fail(c, gerr)
		}
		code, msg := classify(err)
		return Fail(c, gftperrors.New(code, msg).WithCause(err))
	}
	return OK(c, result)
}

type sessionDirectoryReader struct {
	client  transfer.Client
	handler *Handler
	session *auth.Session
}

func (r *sessionDirectoryReader) lock(ctx context.Context) error {
	if err := r.session.LockTransferContext(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		r.session.UnlockTransfer()
		return err
	}
	_, alive := r.handler.store.Get(r.session.ID)
	client, present := r.session.Get("client")
	if !alive || !present || client != r.client {
		r.session.UnlockTransfer()
		return gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection").WithCause(transfer.ErrDirectorySizeUnavailable)
	}
	return nil
}

func (r *sessionDirectoryReader) LstatContext(ctx context.Context, p string) (transfer.FileInfo, error) {
	if err := r.lock(ctx); err != nil {
		return transfer.FileInfo{}, err
	}
	defer r.session.UnlockTransfer()
	info, err := r.client.LstatContext(ctx, p)
	return info, r.check(ctx, err)
}

func (r *sessionDirectoryReader) ReadDirContext(ctx context.Context, p string, limit int) ([]transfer.FileInfo, bool, error) {
	if err := r.lock(ctx); err != nil {
		return nil, false, err
	}
	defer r.session.UnlockTransfer()
	entries, truncated, err := r.client.ReadDirContext(ctx, p, limit)
	return entries, truncated, r.check(ctx, err)
}

func (r *sessionDirectoryReader) check(ctx context.Context, err error) error {
	if isConnLost(err) {
		closeSessionClient(r.session)
		return fmt.Errorf("%w: %w", transfer.ErrConnectionLost, err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

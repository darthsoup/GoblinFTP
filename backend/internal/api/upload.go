// backend/internal/api/upload.go
package api

import (
	"errors"
	"path"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/metrics"
	"github.com/darthsoup/goblinftp/internal/staging"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// stagingError maps chunk-store failures to API errors: connection-level
// storage outages become ERR_STORAGE_UNAVAILABLE (503), everything else
// keeps the handler's usual code.
func stagingError(err error, fallback gftperrors.Code, msg string) *gftperrors.GFTPError {
	if errors.Is(err, staging.ErrUnavailable) {
		return gftperrors.New(gftperrors.ErrStorageUnavailable, "chunk storage unavailable")
	}
	return gftperrors.New(fallback, msg)
}

// destinationTaken reports whether p already exists on the remote. An ordinary
// Stat failure means "free", matching how ensureDirAll and copyTree infer
// non-existence, but a dead connection is surfaced as err so the caller fails
// the request instead of assuming the path is free and clobbering it.
// Callers must already hold the per-session transfer lock.
func destinationTaken(client transfer.Client, p string) (taken, isDir bool, err error) {
	fi, statErr := client.Stat(p)
	if statErr == nil {
		return true, fi.IsDir, nil
	}
	if isConnLost(statErr) {
		return false, false, statErr
	}
	return false, false, nil
}

// probeDestination reports whether p is taken, taking and releasing the
// per-session transfer lock itself. The deferred release matters: reserve holds
// the lock only for this probe, and an explicit release would strand it - and
// with it every later request on the session - if the probe panicked.
func probeDestination(c echo.Context, p string) (taken, connected bool, err error) {
	client, release, ok := lockedClient(c)
	if !ok {
		return false, false, nil
	}
	defer release()
	taken, _, err = destinationTaken(client, p)
	return taken, true, err
}

func (h *Handler) UploadSimple(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	remotePath := c.FormValue("path")
	if remotePath == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "path is required"))
	}
	overwrite, _ := strconv.ParseBool(c.FormValue("overwrite"))
	fh, err := c.FormFile("file")
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "file is required"))
	}
	f, err := fh.Open()
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "failed to open file"))
	}
	defer f.Close()
	sess, _ := c.Get("session").(*auth.Session)
	// Runs before the existence probe so a freshly created parent can skip it.
	created, err := ensureDirAllCreated(client, path.Dir(remotePath))
	if err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	// Echo has already parsed the multipart body by now, so this 409 is a safety
	// net rather than a bandwidth saver - /files/upload/check is what lets the
	// client resolve conflicts before sending anything.
	if !overwrite && !created {
		taken, _, err := destinationTaken(client, remotePath)
		if err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		if taken {
			return Fail(c, gftperrors.New(gftperrors.ErrFileExists, "destination already exists"))
		}
	}
	src := metrics.CountingReader(f, h.metrics.TransferBytes.WithLabelValues("upload", protocolFromSession(sess)))
	if err := client.Upload(remotePath, src); err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	return OK(c, nil)
}

func (h *Handler) UploadReserve(c echo.Context) error {
	sess, ok := c.Get("session").(*auth.Session)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active session"))
	}
	var req struct {
		Path        string `json:"path"`
		TotalChunks int    `json:"totalChunks"`
		TotalSize   int64  `json:"totalSize"`
		ChunkSize   int64  `json:"chunkSize"`
		Overwrite   bool   `json:"overwrite"`
	}
	if err := c.Bind(&req); err != nil || req.Path == "" || req.TotalChunks < 1 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "path and totalChunks are required"))
	}
	// Reject a conflict here, before the client stages a single chunk. The lock
	// is required because a LIST issued mid-transfer would desync the shared
	// FTP control connection; it is released before the chunk-store call, which
	// has nothing to do with the remote.
	if !req.Overwrite {
		taken, connected, err := probeDestination(c, req.Path)
		if !connected {
			return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
		}
		if err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		if taken {
			return Fail(c, gftperrors.New(gftperrors.ErrFileExists, "destination already exists"))
		}
	}
	meta, err := h.chunks.NewUpload(c.Request().Context(), req.Path, req.TotalChunks, req.ChunkSize)
	if err != nil {
		return Fail(c, stagingError(err, gftperrors.ErrInternal, "failed to reserve upload"))
	}
	meta.Overwrite = req.Overwrite
	sess.PutUpload(meta.ID, meta)
	return OK(c, map[string]string{"uploadId": meta.ID})
}

func (h *Handler) UploadChunk(c echo.Context) error {
	sess, ok := c.Get("session").(*auth.Session)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active session"))
	}
	uploadID := c.FormValue("uploadId")
	chunkIndexStr := c.FormValue("chunkIndex")
	if uploadID == "" || chunkIndexStr == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "uploadId and chunkIndex are required"))
	}
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "invalid chunkIndex"))
	}
	metaVal, ok := sess.GetUpload(uploadID)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUploadNotFound, "upload not found"))
	}
	meta, ok := metaVal.(*transfer.UploadMeta)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUploadNotFound, "upload not found"))
	}
	if chunkIndex < 0 || chunkIndex >= meta.TotalChunks {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "chunkIndex out of range"))
	}
	fh, err := c.FormFile("chunk")
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "chunk file is required"))
	}
	f, err := fh.Open()
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "failed to open chunk"))
	}
	defer f.Close()
	if err := h.chunks.WriteChunk(c.Request().Context(), uploadID, chunkIndex, fh.Size, f); err != nil {
		return Fail(c, stagingError(err, gftperrors.ErrInternal, "failed to write chunk"))
	}
	return OK(c, nil)
}

func (h *Handler) UploadCommit(c echo.Context) error {
	sess, ok := c.Get("session").(*auth.Session)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active session"))
	}
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	var req struct {
		UploadID string `json:"uploadId"`
		// nil inherits the consent given at reserve time; an explicit value lets
		// the client resolve a conflict raised by an earlier commit attempt.
		Overwrite *bool `json:"overwrite"`
		// Optional rename, restricted to the reserved parent directory.
		Destination string `json:"destination"`
	}
	if err := c.Bind(&req); err != nil || req.UploadID == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "uploadId is required"))
	}
	metaVal, ok := sess.GetUpload(req.UploadID)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUploadNotFound, "upload not found"))
	}
	meta, ok := metaVal.(*transfer.UploadMeta)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUploadNotFound, "upload not found"))
	}
	destination := meta.Destination
	if req.Destination != "" {
		// Confining the rename to the reserved directory keeps an uploadId from
		// being retargeted anywhere else on the server.
		if path.Dir(path.Clean(req.Destination)) != path.Dir(destination) {
			return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "destination must stay in the reserved directory"))
		}
		destination = path.Clean(req.Destination)
	}
	overwrite := meta.Overwrite
	if req.Overwrite != nil {
		overwrite = *req.Overwrite
	}
	ctx := c.Request().Context()
	// Probed before assembling so a conflict costs no reader setup. A conflict
	// deliberately leaves the staged chunks in place so the client can re-commit
	// with overwrite or a new destination instead of re-uploading; /upload/abort
	// is how it discards them after choosing to skip.
	if !overwrite {
		taken, _, err := destinationTaken(client, destination)
		if err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		if taken {
			return Fail(c, gftperrors.New(gftperrors.ErrFileExists, "destination already exists"))
		}
	}
	r, err := h.chunks.AssembleReader(ctx, meta.ID, meta.TotalChunks)
	if err != nil {
		return Fail(c, stagingError(err, gftperrors.ErrInternal, "failed to assemble chunks"))
	}
	defer r.Close()
	if err := ensureDirAll(client, path.Dir(destination)); err != nil {
		_ = h.chunks.Cleanup(ctx, meta.ID)
		sess.DeleteUpload(req.UploadID)
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	src := metrics.CountingReader(r, h.metrics.TransferBytes.WithLabelValues("upload", protocolFromSession(sess)))
	if err := client.Upload(destination, src); err != nil {
		// The frontend never retries a failed commit, so the staged chunks
		// are unreachable - clean them up instead of leaving them behind.
		_ = h.chunks.Cleanup(ctx, meta.ID)
		sess.DeleteUpload(req.UploadID)
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	_ = h.chunks.Cleanup(ctx, meta.ID)
	sess.DeleteUpload(req.UploadID)
	return OK(c, nil)
}

// UploadAbort discards a reserved upload and its staged chunks. Nothing sweeps
// staging, so a client that walks away from a commit conflict must call this or
// the chunks live until the volume fills.
func (h *Handler) UploadAbort(c echo.Context) error {
	sess, ok := c.Get("session").(*auth.Session)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active session"))
	}
	var req struct {
		UploadID string `json:"uploadId"`
	}
	if err := c.Bind(&req); err != nil || req.UploadID == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "uploadId is required"))
	}
	metaVal, ok := sess.GetUpload(req.UploadID)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUploadNotFound, "upload not found"))
	}
	meta, ok := metaVal.(*transfer.UploadMeta)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUploadNotFound, "upload not found"))
	}
	if err := h.chunks.Cleanup(c.Request().Context(), meta.ID); err != nil {
		return Fail(c, stagingError(err, gftperrors.ErrInternal, "failed to discard staged chunks"))
	}
	sess.DeleteUpload(req.UploadID)
	return OK(c, nil)
}

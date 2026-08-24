package api

import (
	"context"
	"errors"
	"log/slog"
	"path"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/metrics"
	"github.com/darthsoup/goblinftp/internal/staging"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// stagingError maps chunk-store failures to API errors: a storage outage becomes
// ERR_STORAGE_UNAVAILABLE (503), everything else keeps the handler's code.
func stagingError(err error, fallback gftperrors.Code, msg string) *gftperrors.GFTPError {
	switch {
	case errors.Is(err, staging.ErrUnavailable):
		return gftperrors.New(gftperrors.ErrStorageUnavailable,
			"temporary storage is unavailable").WithCause(err)
	case errors.Is(err, staging.ErrChunkMissing):
		// The upload is stale or already cleaned up, so the caller must start
		// over. Reporting it as an internal fault invited an endless retry.
		return gftperrors.New(gftperrors.ErrUploadNotFound,
			"the staged upload is no longer available, please upload again").WithCause(err)
	}
	return gftperrors.New(fallback, msg).WithCause(err)
}

// destinationTaken reports whether p exists; callers must hold the transfer lock.
// It fails closed: only a definite ErrNotFound counts as free.
func destinationTaken(client transfer.Client, p string) (taken, isDir bool, err error) {
	fi, statErr := client.Stat(p)
	if statErr == nil {
		return true, fi.IsDir, nil
	}
	// Only a definite "not there" clears the overwrite guard. Treating every
	// other Stat failure as free let a dropped LIST entry clobber a real file.
	if errors.Is(statErr, transfer.ErrNotFound) {
		return false, false, nil
	}
	return true, false, statErr
}

// probeDestination reports whether p is taken, taking and releasing the transfer
// lock itself. Deferred release: an explicit one would strand it on a panic.
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
	// Echo has already parsed the multipart body, so this 409 is a safety net;
	// /files/upload/check is what lets the client resolve conflicts up front.
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
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if req.Path == "" || req.TotalChunks < 1 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "path and totalChunks are required"))
	}
	if req.TotalChunks > maxTotalChunks {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "too many chunks for one upload"))
	}
	if req.ChunkSize <= 0 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "chunkSize must be positive"))
	}
	if req.TotalSize < 0 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "totalSize must not be negative"))
	}
	// Reject a conflict before the client stages a single chunk. The lock is
	// required because a LIST issued mid-transfer desyncs the FTP control connection.
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
	meta.TotalSize = req.TotalSize
	sess.PutUpload(meta.ID, meta)
	return OK(c, map[string]string{"uploadId": meta.ID})
}

// maxTotalChunks bounds a single reservation: the count is client-supplied and
// drives an allocation in AssembleReader.
const maxTotalChunks = 100_000

// checkChunkSize rejects a chunk that cannot belong to the reserved upload.
// Only the final chunk may be short, and none may exceed the reserved size.
func checkChunkSize(meta *transfer.UploadMeta, index int, size int64) *gftperrors.GFTPError {
	if meta.ChunkSize <= 0 || size < 0 {
		return nil
	}
	if size > meta.ChunkSize {
		return gftperrors.New(gftperrors.ErrBadRequest, "chunk is larger than the reserved chunk size")
	}
	if size < meta.ChunkSize && index != meta.TotalChunks-1 {
		return gftperrors.New(gftperrors.ErrBadRequest, "only the final chunk may be short")
	}
	return nil
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
	if gerr := checkChunkSize(meta, chunkIndex, fh.Size); gerr != nil {
		return Fail(c, gerr)
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
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if req.UploadID == "" {
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
	// Probed before assembling so a conflict costs no reader setup, and the staged
	// chunks survive it so the client can re-commit rather than re-upload.
	createdHere := false
	if !overwrite {
		taken, _, err := destinationTaken(client, destination)
		if err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		if taken {
			return Fail(c, gftperrors.New(gftperrors.ErrFileExists, "destination already exists"))
		}
		createdHere = true
	}
	r, err := h.chunks.AssembleReader(ctx, meta.ID, meta.TotalChunks)
	if err != nil {
		return Fail(c, stagingError(err, gftperrors.ErrInternal, "failed to assemble chunks"))
	}
	defer r.Close()
	if err := ensureDirAll(client, path.Dir(destination)); err != nil {
		h.cleanupChunks(meta.ID)
		sess.DeleteUpload(req.UploadID)
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	src := metrics.CountingReader(r, h.metrics.TransferBytes.WithLabelValues("upload", protocolFromSession(sess)))
	if err := client.Upload(destination, src); err != nil {
		// The frontend never retries a failed commit, so the staged chunks are
		// unreachable: clean them up instead of leaving them behind.
		h.cleanupChunks(meta.ID)
		sess.DeleteUpload(req.UploadID)
		// A half-written file we created ourselves is pure litter. One we were
		// overwriting is the user's data, so a partial write is reported, never
		// deleted.
		if createdHere && errors.Is(err, transfer.ErrTransferIncomplete) {
			_ = client.Delete(destination)
			return failClient(c, gftperrors.ErrTransferIncomplete, err)
		}
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	h.cleanupChunks(meta.ID)
	sess.DeleteUpload(req.UploadID)
	return OK(c, nil)
}

// cleanupChunks reclaims staged chunks on a fresh context: the request context
// is already canceled whenever the client hung up mid-commit.
func (h *Handler) cleanupChunks(uploadID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.chunks.Cleanup(ctx, uploadID); err != nil {
		h.logger.Warn("failed to clean up staged chunks",
			slog.String("upload_id", uploadID), slog.String("error", err.Error()))
	}
}

// UploadAbort discards a reserved upload and its staged chunks. Worth calling:
// staging.Sweeper only reclaims dirs untouched for 24h, an abort frees space now.
func (h *Handler) UploadAbort(c echo.Context) error {
	sess, ok := c.Get("session").(*auth.Session)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active session"))
	}
	var req struct {
		UploadID string `json:"uploadId"`
	}
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if req.UploadID == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "uploadId is required"))
	}
	// Serialized behind a running commit: without the lock an abort deleted the
	// chunks out from under the commit's reader and truncated the remote file.
	// By the time the lock is free a finished commit has already removed the
	// upload, so this falls through to the ordinary not-found below.
	sess.LockTransfer()
	defer sess.UnlockTransfer()

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

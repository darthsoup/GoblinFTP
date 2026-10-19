package api

import (
	"context"
	"time"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// Callers hold the transfer lock while closing a session's protocol client.
func closeSessionClient(sess *auth.Session) {
	value, ok := sess.Get("client")
	if !ok {
		return
	}
	if client, ok := value.(transfer.Client); ok {
		_ = client.Close()
	}
	sess.Delete("client")
}

func (h *Handler) evictSession(sess *auth.Session) {
	closeSessionClient(sess)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, id := range sess.UploadIDs() {
		value, ok := sess.GetUpload(id)
		if !ok {
			continue
		}
		meta, ok := value.(*transfer.UploadMeta)
		if !ok {
			continue
		}
		meta.Mu.Lock()
		meta.Closed = true
		if err := h.chunks.Cleanup(ctx, meta.ID); err != nil {
			h.pendingCleanup.Store(meta.ID, meta)
			h.logger.Warn("could not discard staged chunks for an evicted session", "upload_id", meta.ID, "error", err.Error())
		} else if meta.Release != nil {
			meta.Release()
		}
		meta.Mu.Unlock()
		sess.DeleteUpload(id)
	}
}

func (h *Handler) retryCleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-h.cleanupContext.Done():
			return
		case <-ticker.C:
			h.pendingCleanup.Range(func(key, value any) bool {
				meta := value.(*transfer.UploadMeta)
				ctx, cancel := context.WithTimeout(h.cleanupContext, 10*time.Second)
				err := h.chunks.Cleanup(ctx, meta.ID)
				cancel()
				if err == nil {
					if meta.Release != nil {
						meta.Release()
					}
					h.pendingCleanup.Delete(key)
				}
				return h.cleanupContext.Err() == nil
			})
		}
	}
}

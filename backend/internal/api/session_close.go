// backend/internal/api/session_close.go
package api

import (
	"context"
	"time"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// closeSessionClient closes the transfer client a session holds, if any.
//
// The caller must already hold the session's transfer lock: both callers reach
// here from a point where the connection must not be in use. Disconnect takes
// it explicitly; the store's eviction sweep takes it before invoking the hook.
func closeSessionClient(sess *auth.Session) {
	clientVal, ok := sess.Get("client")
	if !ok {
		return
	}
	if client, ok := clientVal.(transfer.Client); ok {
		_ = client.Close()
	}
	sess.Delete("client")
}

// evictSession is the store's eviction hook: it releases everything a session
// owns. Registered in newHandler so an expired session frees its connection and
// its staged upload chunks, not just its map entry.
func (h *Handler) evictSession(sess *auth.Session) {
	closeSessionClient(sess)

	ids := sess.UploadIDs()
	if len(ids) == 0 {
		return
	}
	// Bounded: eviction runs on the janitor goroutine (and at shutdown), so a
	// slow or unreachable chunk store must not stall it.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, id := range ids {
		metaVal, ok := sess.GetUpload(id)
		if !ok {
			continue
		}
		meta, ok := metaVal.(*transfer.UploadMeta)
		if !ok {
			continue
		}
		if err := h.chunks.Cleanup(ctx, meta.ID); err != nil {
			h.logger.Warn("could not discard staged chunks for an evicted session",
				"upload_id", meta.ID, "error", err.Error())
		}
		sess.DeleteUpload(id)
	}
}

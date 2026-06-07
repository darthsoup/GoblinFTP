// backend/internal/api/handler.go
package api

import (
	"log/slog"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/sso"
	"github.com/darthsoup/goblinftp/internal/staging"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// DialFunc creates a transfer.Client for the given protocol, address, credentials, and passive flag.
type DialFunc func(protocol, addr, user, pass string, passive bool) (transfer.Client, error)

// HandlerOption is a functional option for constructing a Handler.
type HandlerOption func(*Handler)

// WithDial overrides the dial function (primarily for testing).
func WithDial(fn DialFunc) HandlerOption {
	return func(h *Handler) {
		h.dial = fn
	}
}

// WithChunkStore overrides the chunk staging backend (S3 in production, mocks in tests).
func WithChunkStore(s staging.ChunkStore) HandlerOption {
	return func(h *Handler) {
		h.chunks = s
	}
}

// WithLogger sets the structured logger used for the access log and the
// frontend-error endpoint (a discard logger is used when unset).
func WithLogger(l *slog.Logger) HandlerOption {
	return func(h *Handler) {
		h.logger = l
	}
}

// Handler holds shared dependencies for all API handlers.
type Handler struct {
	cfg      *config.Config
	store    *auth.Store
	throttle *auth.Throttle
	chunks   staging.ChunkStore
	dial     DialFunc
	ssoUsed  *sso.UsedSet
	logger   *slog.Logger
	// frontendLog rate-limits /api/log/frontend per client IP — deliberately
	// separate from the login throttle so report spam cannot lock out logins.
	frontendLog *auth.Throttle
}

func newHandler(cfg *config.Config, store *auth.Store, thr *auth.Throttle, opts []HandlerOption) *Handler {
	h := &Handler{
		cfg:         cfg,
		store:       store,
		throttle:    thr,
		chunks:      staging.NewLocalStore(cfg.DataDir),
		dial:        defaultDial,
		ssoUsed:     sso.NewUsedSet(),
		logger:      slog.New(slog.DiscardHandler),
		frontendLog: auth.NewThrottle(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

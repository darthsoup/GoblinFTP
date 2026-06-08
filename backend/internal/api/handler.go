// backend/internal/api/handler.go
package api

import (
	"log/slog"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/metrics"
	"github.com/darthsoup/goblinftp/internal/sso"
	"github.com/darthsoup/goblinftp/internal/staging"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// DialRequest carries the per-connection inputs for a dial attempt.
type DialRequest struct {
	Protocol string
	Addr     string // host:port
	Host     string // bare host (TLS SNI for FTPS)
	User     string
	Pass     string
	Passive  bool
	// AcceptHostKey, when non-empty, is the SHA256 fingerprint the user agreed
	// to trust for an unknown SFTP host (trust-on-first-use, step 2).
	AcceptHostKey string
}

// HostKeyPrompt is returned (with a nil client and nil error) when an SFTP host
// key must be confirmed by the user before the connection can proceed.
type HostKeyPrompt struct {
	Fingerprint string `json:"fingerprint"`
	KeyType     string `json:"keyType"`
}

// DialFunc creates a transfer.Client. It returns (client, nil, nil) on success,
// (nil, prompt, nil) when an SFTP host key needs user confirmation, and
// (nil, nil, err) on failure.
type DialFunc func(DialRequest) (transfer.Client, *HostKeyPrompt, error)

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

// WithMetrics overrides the metrics instance. main.go shares its registry
// with the dedicated /metrics listener; tests assert against a known registry.
// newHandler wires the session-store snapshot into whichever instance is active.
func WithMetrics(m *metrics.Metrics) HandlerOption {
	return func(h *Handler) {
		h.metrics = m
	}
}

// WithVersion sets the build version surfaced in /healthz and /api/system/vars
// ("dev" when unset — release builds inject the tag via ldflags in main).
func WithVersion(v string) HandlerOption {
	return func(h *Handler) {
		h.version = v
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
	metrics  *metrics.Metrics
	version  string
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
		dial:        newDefaultDial(cfg),
		ssoUsed:     sso.NewUsedSet(),
		logger:      slog.New(slog.DiscardHandler),
		metrics:     metrics.New(),
		version:     "dev",
		frontendLog: auth.NewThrottle(),
	}
	for _, opt := range opts {
		opt(h)
	}
	// Wire the scrape-time gauges into whichever metrics instance is active
	// (the default above, or one supplied via WithMetrics).
	h.metrics.SetConnectionSnapshot(h.connectionSnapshot)
	return h
}

// connectionSnapshot is the scrape-time view of the session store: live
// sessions, and those holding a transfer client grouped by protocol. The TTL
// cleanup drops expired sessions without closing the underlying connection —
// a session is deliberately the unit counted here.
func (h *Handler) connectionSnapshot() metrics.Snapshot {
	snap := metrics.Snapshot{ConnsByProtocol: map[string]int{"ftp": 0, "ftps": 0, "sftp": 0}}
	h.store.Range(func(sess *auth.Session) {
		snap.Sessions++
		// "client" is only ever set to a live transfer.Client (and deleted on
		// disconnect / connection loss), so key presence == has a connection.
		if _, hasClient := sess.Get("client"); hasClient {
			if proto := sess.GetString("protocol"); proto == "ftp" || proto == "ftps" || proto == "sftp" {
				snap.ConnsByProtocol[proto]++
			}
		}
	})
	return snap
}

// protocolFromSession returns the connection protocol stored at connect time,
// used as a metrics label ("ftp"/"sftp", "unknown" if absent).
func protocolFromSession(sess *auth.Session) string {
	if sess == nil {
		return "unknown"
	}
	if p := sess.GetString("protocol"); p == "ftp" || p == "ftps" || p == "sftp" {
		return p
	}
	return "unknown"
}

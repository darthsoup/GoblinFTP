// backend/internal/api/router.go
package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
)

const (
	// SessionCookieName is the cookie name used to identify sessions.
	SessionCookieName = "gftp_session"

	// jsonBodyLimit bounds ordinary JSON bodies. Without it c.Bind reads an
	// arbitrarily large body into memory, and /api/auth/connect is reachable
	// unauthenticated. Applied per route rather than on the group: a group
	// limit is a hard ceiling that route middleware cannot raise, and three
	// endpoints legitimately carry far more.
	jsonBodyLimit = "1M"
	// editorBodyLimit sits above the handler's own 1 MB check (editor.go) so
	// an oversized edit still gets the enveloped ERR_FILE_TOO_LARGE rather than
	// a bare 413; this is only the backstop for abusive bodies.
	editorBodyLimit = "4M"
	// archiveBodyLimit matches maxZipSize, the extract handler's own cap.
	archiveBodyLimit = "512M"
)

// chunkBodyLimit sizes the upload endpoints from the configured chunk size,
// with headroom for multipart framing and the metadata fields.
func chunkBodyLimit(cfg *config.Config) string {
	mb := cfg.ChunkSize/(1024*1024) + 8
	return strconv.FormatInt(mb, 10) + "M"
}

// Register adds the /healthz route, global middleware, and all /api/* routes to
// e, and returns the handler so its owner can Close it at shutdown.
func Register(e *echo.Echo, cfg *config.Config, store *auth.Store, thr *auth.Throttle, opts ...HandlerOption) *Handler {
	h := newHandler(cfg, store, thr, opts)

	// Order matters: RequestID before the access logger (the line carries the
	// ID); metrics outside the logger (whose c.Error commits echo-level
	// errors before metrics reads the status); the logger above Recover
	// (a recovered panic still logs as a 500).
	e.Use(middleware.RequestID())
	e.Use(metricsMiddleware(h.metrics))
	e.Use(requestLogger(h.logger))
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		// Route the panic + stack into the structured logger instead of
		// echo's plain-text [PANIC RECOVER] print; returning err lets the
		// default error handler commit the 500 as usual.
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			h.logger.LogAttrs(c.Request().Context(), slog.LevelError, "panic recovered",
				slog.String("error", err.Error()),
				slog.String("stack", string(stack)),
			)
			return err
		},
	}))
	e.Use(securityHeadersMiddleware(cfg))

	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok", "version": h.version})
	})

	// SSO entry point and auth status (public, no CSRF required)
	e.GET("/", h.SSOLogin)
	e.GET("/api/auth/status", h.AuthStatus)

	// Public routes (no auth required)
	e.GET("/api/system/vars", h.SystemVars)
	e.GET("/api/files/download", h.DownloadFile)

	// Per-tenant white-label theme assets (public; params allowlisted in-handler).
	e.GET("/themes/:tenant/:file", h.ServeTheme)

	// Browser-error forwarding (public, no CSRF - login-screen errors happen
	// before any session exists; throttled per IP inside the handler)
	e.POST("/api/log/frontend", h.FrontendLog, middleware.BodyLimit("16K"))

	apiGroup := e.Group("/api")
	// INVARIANT: no CORS middleware may be added here. Under an embed
	// deployment the session cookie is SameSite=None, so the only thing
	// stopping a cross-origin page from reading the CSRF token out of
	// GET /api/auth/status is the absence of Access-Control-Allow-Origin.
	// TestNoCORSHeadersEver guards this.
	apiGroup.Use(csrfMiddleware(store))

	// Auth
	apiGroup.POST("/auth/connect", h.Connect, middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/auth/disconnect", requireSession(store)(h.Disconnect), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/auth/sso-connect", requireSession(store)(h.SSOConnect), middleware.BodyLimit(jsonBodyLimit))

	// File operations - Phase 3
	apiGroup.GET("/files", requireSession(store)(h.ListFiles))
	apiGroup.POST("/files/directory", requireSession(store)(h.CreateDirectory), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.DELETE("/files", requireSession(store)(h.DeleteFiles), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.PATCH("/files/rename", requireSession(store)(h.RenameFile), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.PATCH("/files/copy", requireSession(store)(h.CopyFile), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.PATCH("/files/permissions", requireSession(store)(h.SetPermissions), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/files/download-token", requireSession(store)(h.IssueDownloadToken), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/files/download-zip", requireSession(store)(h.DownloadZip), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/files/upload", requireSession(store)(h.UploadSimple),
		middleware.BodyLimit(chunkBodyLimit(cfg)))
	apiGroup.POST("/files/upload/check", requireSession(store)(h.UploadCheck), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/files/upload/reserve", requireSession(store)(h.UploadReserve), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/files/upload/abort", requireSession(store)(h.UploadAbort), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/files/upload/chunk", requireSession(store)(h.UploadChunk),
		middleware.BodyLimit(chunkBodyLimit(cfg)))
	apiGroup.POST("/files/upload/commit", requireSession(store)(h.UploadCommit), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/files/extract", requireSession(store)(h.ExtractArchive),
		middleware.BodyLimit(archiveBodyLimit))
	apiGroup.POST("/files/compress", requireSession(store)(h.CreateZip), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.GET("/files/read", requireSession(store)(h.ReadFile))
	apiGroup.POST("/files/write", requireSession(store)(h.WriteFile),
		middleware.BodyLimit(editorBodyLimit))

	return h
}

// securityHeadersMiddleware sets the app CSP, including the frame-ancestors
// allowlist, on every response the Go binary produces.
//
// Everything goes in ONE header: a second Content-Security-Policy response
// header is enforced as the intersection of both policies, which is correct but
// near-impossible to debug from a browser console.
//
// This covers only Go's own responses. In production Caddy serves index.html -
// the document frame-ancestors actually applies to - and emits the same
// directive from the same env var (docker/Caddyfile). Both must agree.
func securityHeadersMiddleware(cfg *config.Config) echo.MiddlewareFunc {
	ancestors := "'none'"
	if cfg.EmbeddingEnabled() {
		ancestors = strings.Join(cfg.FrameAncestors, " ")
	}
	csp := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; frame-ancestors " + ancestors

	// X-Frame-Options is emitted ONLY in the deny case. It cannot express an
	// allowlist (ALLOW-FROM is gone from every shipping engine), so an engine
	// that prefers it over frame-ancestors would silently override the
	// allowlist. Restricting it to the deny case means it can only ever agree
	// with frame-ancestors 'none'.
	denyFraming := !cfg.EmbeddingEnabled()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("Content-Security-Policy", csp)
			// The app serves back file content the user does not control the
			// type of, plus operator-supplied theme SVGs from this origin, so
			// content-type sniffing must be off.
			h.Set("X-Content-Type-Options", "nosniff")
			// no-referrer rather than the usual strict-origin: the SSO token
			// (which decrypts to an FTP password) and download tokens ride in
			// query strings, and Referer would carry them off-site.
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			if denyFraming {
				h.Set("X-Frame-Options", "DENY")
			}
			return next(c)
		}
	}
}

// csrfMiddleware validates CSRF tokens on all state-changing requests within the /api group.
// Skips /api/auth/connect (unauthenticated) and read-only methods (GET, HEAD, OPTIONS).
func csrfMiddleware(store *auth.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			method := c.Request().Method
			if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
				return next(c)
			}
			if c.Path() == "/api/auth/connect" {
				// Unauthenticated, so there is no token to check yet. Echo's
				// binder ignores form fields without a `form` tag, so a
				// cross-site simple-request POST already binds nothing - but
				// that is an implementation detail of a library, not a
				// guarantee. Fetch metadata makes the intent explicit and is
				// sent by every browser that supports SameSite=None.
				if c.Request().Header.Get("Sec-Fetch-Site") == "cross-site" {
					return Fail(c, gftperrors.New(gftperrors.ErrForbidden, "cross-site connect is not allowed"))
				}
				return next(c)
			}

			if !hasSessionCookie(c) {
				return Fail(c, gftperrors.New(gftperrors.ErrCSRFInvalid, "missing session cookie"))
			}
			sess, ok := lookupSession(c, store)
			if !ok {
				return Fail(c, gftperrors.New(gftperrors.ErrCSRFInvalid, "invalid or expired session"))
			}
			storedToken := sess.GetString(auth.CSRFSessionKey)
			headerToken := c.Request().Header.Get(auth.CSRFHeaderName)
			if !auth.ValidateCSRFToken(storedToken, headerToken) {
				return Fail(c, gftperrors.New(gftperrors.ErrCSRFInvalid, "CSRF token mismatch"))
			}
			return next(c)
		}
	}
}

// requireSession returns Echo middleware that enforces a valid session cookie.
// It stores the *auth.Session in the Echo context under key "session".
func requireSession(store *auth.Store) func(echo.HandlerFunc) echo.HandlerFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !hasSessionCookie(c) {
				return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "not authenticated"))
			}
			sess, ok := lookupSession(c, store)
			if !ok {
				return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "session expired or not found"))
			}
			store.Touch(sess.ID)
			c.Set("session", sess)
			return next(c)
		}
	}
}

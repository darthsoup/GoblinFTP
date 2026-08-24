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
	gftpsentry "github.com/darthsoup/goblinftp/internal/sentry"
)

const (
	// SessionCookieName is the cookie name used to identify sessions.
	SessionCookieName = "gftp_session"

	// jsonBodyLimit bounds ordinary JSON bodies (c.Bind would read any size, and
	// connect is unauthenticated). Per route: a group limit no route could raise.
	jsonBodyLimit = "1M"
	// editorBodyLimit sits above the handler's own 1 MB check (editor.go) so an
	// oversized edit gets the enveloped ERR_FILE_TOO_LARGE, not a bare 413.
	editorBodyLimit = "4M"
	// archiveBodyLimit sits above maxZipSize so the extract handler's own check
	// wins and returns an enveloped ERR_ARCHIVE_FORMAT, not a bare 413.
	archiveBodyLimit = "544M"
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

	// Installed here, not in main, so newTestApp exercises the same behavior.
	e.HTTPErrorHandler = httpErrorHandler(h.logger)

	// Order matters: RequestID before the logger (the line carries the ID) and
	// before Sentry (the event carries it), Sentry outside the logger (whose
	// c.Error commits the status Sentry reads), metrics outside the logger too
	// (its c.Error commits first), the logger above Recover.
	e.Use(middleware.RequestID())
	e.Use(gftpsentry.Middleware(sentryMiddlewareConfig(cfg)))
	e.Use(metricsMiddleware(h.metrics))
	e.Use(requestLogger(h.logger))
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		// Route the panic and stack into the structured logger instead of echo's
		// plain-text print; returning err lets the default handler commit the 500.
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			h.logger.LogAttrs(c.Request().Context(), slog.LevelError, "panic recovered",
				slog.String("error", err.Error()),
				slog.String("stack", string(stack)),
			)
			// Recover calls c.Error and leaves the return nil, so the panic never
			// reaches an outer defer. This hook is the only place Sentry can see it.
			gftpsentry.CapturePanic(c, err, stack)
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

	// Browser-error forwarding (public, no CSRF: login-screen errors happen before
	// any session exists; throttled per IP inside the handler)
	e.POST("/api/log/frontend", h.FrontendLog, middleware.BodyLimit("16K"))

	apiGroup := e.Group("/api")
	// INVARIANT: no CORS middleware here (TestNoCORSHeadersEver guards it). Under
	// embed the cookie is SameSite=None, so only a missing ACAO hides the CSRF token.
	apiGroup.Use(csrfMiddleware(store))

	apiGroup.POST("/auth/connect", h.Connect, middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/auth/disconnect", requireSession(store)(h.Disconnect), middleware.BodyLimit(jsonBodyLimit))
	apiGroup.POST("/auth/sso-connect", requireSession(store)(h.SSOConnect), middleware.BodyLimit(jsonBodyLimit))

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

// securityHeadersMiddleware sets the app CSP (frame-ancestors included) on Go's
// own responses. Caddy emits the same directive for index.html; both must agree.
func securityHeadersMiddleware(cfg *config.Config) echo.MiddlewareFunc {
	ancestors := "'none'"
	if cfg.EmbeddingEnabled() {
		ancestors = strings.Join(cfg.FrameAncestors, " ")
	}
	// One header only: a second Content-Security-Policy is enforced as the
	// intersection of both, correct but near-impossible to debug in a browser.
	csp := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; frame-ancestors " + ancestors

	// X-Frame-Options ONLY in the deny case: it cannot express an allowlist, so an
	// engine preferring it over frame-ancestors would silently override one.
	denyFraming := !cfg.EmbeddingEnabled()

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("Content-Security-Policy", csp)
			// The app serves user file content and operator theme SVGs from this
			// origin, so content-type sniffing must be off.
			h.Set("X-Content-Type-Options", "nosniff")
			// no-referrer rather than strict-origin: SSO tokens (which decrypt to an
			// FTP password) and download tokens ride in query strings.
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
				// Unauthenticated, so there is no token to check yet. Fetch metadata
				// states the intent explicitly and every SameSite=None browser sends it.
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

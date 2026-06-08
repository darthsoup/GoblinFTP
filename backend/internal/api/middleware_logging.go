// backend/internal/api/middleware_logging.go
package api

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/logging"
)

// requestLogger emits exactly one structured log line per request. Handlers
// write failures through Fail (which commits the envelope and returns nil), so
// the committed response status is authoritative and the GFTPError is read
// back from the context (LoggedErrorKey) rather than the return value.
//
// It must be registered above middleware.Recover so recovered panics still
// produce a logged 500 line, and below middleware.RequestID so the ID exists.
func requestLogger(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			if err != nil {
				// Echo-level errors (404, 405, BodyLimit 413, …) bypass Fail;
				// commit them now so the line carries the real status. The
				// global error handler is idempotent on committed responses,
				// so returning err up the chain stays safe.
				c.Error(err)
			}

			req := c.Request()
			res := c.Response()
			attrs := []slog.Attr{
				slog.String("method", req.Method),
				slog.String("path", req.URL.Path),
				slog.Int("status", res.Status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("request_id", res.Header().Get(echo.HeaderXRequestID)),
				slog.String("remote_ip", c.RealIP()),
				slog.Int64("bytes_out", res.Size),
			}
			if sess, ok := c.Get("session").(*auth.Session); ok && sess != nil {
				// Only a short prefix: the full session ID is the bearer cookie value.
				if len(sess.ID) >= 8 {
					attrs = append(attrs, slog.String("session", sess.ID[:8]))
				}
				if user := sess.GetString("username"); user != "" {
					attrs = append(attrs, slog.String("user", user))
				}
				if host := sess.GetString("host"); host != "" {
					attrs = append(attrs, slog.String("host", host))
				}
			}
			if ge, ok := c.Get(LoggedErrorKey).(*gftperrors.GFTPError); ok && ge != nil {
				attrs = append(attrs,
					slog.String("error_code", string(ge.Code())),
					slog.String("error", ge.Error()),
				)
				if cause := ge.Unwrap(); cause != nil && cause.Error() != ge.Error() {
					attrs = append(attrs, logging.SafeLogAttrs(slog.String("cause", cause.Error()))...)
				}
			}

			level := slog.LevelInfo
			switch {
			case req.URL.Path == "/healthz":
				level = slog.LevelDebug // polled by the container entrypoint
			case res.Status >= 500:
				level = slog.LevelError
			case res.Status >= 400:
				level = slog.LevelWarn
			}
			logger.LogAttrs(req.Context(), level, "request", attrs...)
			return err
		}
	}
}

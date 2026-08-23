package api

import (
	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	gftpsentry "github.com/darthsoup/goblinftp/internal/sentry"
)

// sentryMiddlewareConfig binds the Sentry middleware to the context keys this
// package owns, so internal/sentry never has to import internal/api.
func sentryMiddlewareConfig(cfg *config.Config) gftpsentry.MiddlewareConfig {
	return gftpsentry.MiddlewareConfig{
		LoggedError: func(c echo.Context) *gftperrors.GFTPError {
			ge, _ := c.Get(LoggedErrorKey).(*gftperrors.GFTPError)
			return ge
		},
		Session:             sentrySessionInfo,
		CaptureRemoteErrors: cfg.SentryCaptureRemoteErrors,
		SendSessionContext:  cfg.SentrySendSessionContext,
	}
}

// sentrySessionInfo mirrors the identity fields of the access log line, so a
// Sentry event and its log line can be matched on more than the request ID.
func sentrySessionInfo(c echo.Context) gftpsentry.SessionInfo {
	sess, ok := c.Get("session").(*auth.Session)
	if !ok || sess == nil {
		return gftpsentry.SessionInfo{}
	}
	info := gftpsentry.SessionInfo{
		Username: sess.GetString("username"),
		Host:     sess.GetString("host"),
		Protocol: sess.GetString("protocol"),
	}
	if len(sess.ID) >= 8 {
		info.IDPrefix = sess.ID[:8]
	}
	return info
}

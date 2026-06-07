// backend/internal/api/frontendlog.go
package api

import (
	"time"

	"github.com/labstack/echo/v4"
)

// frontendLogPayload is the browser-error report sent by the SPA's
// error-reporter plugin. All fields are untrusted input.
type frontendLogPayload struct {
	Kind    string `json:"kind"` // error | rejection | vue
	Message string `json:"message"`
	Stack   string `json:"stack"`
	Source  string `json:"source"`
	Route   string `json:"route"`
}

const (
	frontendLogMaxPerMinute = 60
	frontendLogMessageMax   = 500
	frontendLogStackMax     = 4000
	frontendLogFieldMax     = 500
)

// FrontendLog accepts browser-side error reports and writes them to the
// central log. It always answers with a success envelope: the reporting client
// is deliberately dumb (fire-and-forget), so throttled, malformed, or disabled
// reports are dropped silently instead of surfacing an error.
//
// The route is registered without CSRF (errors on the login screen happen
// before any session exists) and with a body limit; abuse is bounded by a
// per-IP throttle that is separate from the login throttle.
func (h *Handler) FrontendLog(c echo.Context) error {
	if !h.cfg.FrontendLogEnabled {
		return OK(c, nil)
	}

	ip := c.RealIP()
	if h.frontendLog.IsThrottled(ip, frontendLogMaxPerMinute) {
		return OK(c, nil)
	}

	var p frontendLogPayload
	if err := c.Bind(&p); err != nil {
		return OK(c, nil)
	}
	switch p.Kind {
	case "error", "rejection", "vue":
	default:
		return OK(c, nil)
	}

	// Record only accepted reports so the throttle window self-heals once a
	// spamming client backs off.
	h.frontendLog.Record(ip, time.Minute)

	h.logger.Warn("frontend error",
		"kind", p.Kind,
		"message", truncate(p.Message, frontendLogMessageMax),
		"stack", truncate(p.Stack, frontendLogStackMax),
		"source", truncate(p.Source, frontendLogFieldMax),
		"route", truncate(p.Route, frontendLogFieldMax),
		"remote_ip", ip,
		"user_agent", c.Request().UserAgent(),
	)
	return OK(c, nil)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

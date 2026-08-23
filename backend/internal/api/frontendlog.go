package api

import (
	"time"

	"github.com/labstack/echo/v4"

	gftpsentry "github.com/darthsoup/goblinftp/internal/sentry"
)

// frontendLogPayload is the browser-error report sent by the SPA's
// error-reporter plugin. All fields are untrusted input.
type frontendLogPayload struct {
	Kind    string `json:"kind"` // error | rejection | vue
	Message string `json:"message"`
	Stack   string `json:"stack"`
	Source  string `json:"source"`
	Route   string `json:"route"`
	// SentryEventID is set when the SPA's own Sentry SDK already captured this
	// error. It is logged for correlation and suppresses the backend relay, so
	// configuring both DSNs does not file the same error twice.
	SentryEventID string `json:"sentryEventId"`
}

const (
	frontendLogMaxPerMinute = 60
	frontendLogMessageMax   = 500
	frontendLogStackMax     = 4000
	frontendLogFieldMax     = 500
	frontendLogEventIDMax   = 64
)

// FrontendLog accepts browser-side error reports. It always answers with a
// success envelope: the reporter is fire-and-forget, so drops stay silent.
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
	h.metrics.FrontendErrors.Inc()

	kind := p.Kind
	message := truncate(p.Message, frontendLogMessageMax)
	stack := truncate(p.Stack, frontendLogStackMax)
	source := truncate(p.Source, frontendLogFieldMax)
	route := truncate(p.Route, frontendLogFieldMax)
	eventID := truncate(p.SentryEventID, frontendLogEventIDMax)

	h.logger.Warn("frontend error",
		"kind", kind,
		"message", message,
		"stack", stack,
		"source", source,
		"route", route,
		"remote_ip", ip,
		"user_agent", c.Request().UserAgent(),
		"sentry_event_id", eventID,
	)

	// Relay only what the SPA could not report itself: with a browser DSN set it
	// already filed the event and sent back its ID, which the log line carries.
	if eventID == "" {
		gftpsentry.CaptureFrontend(gftpsentry.FrontendError{
			Kind:      kind,
			Message:   message,
			Stack:     stack,
			Source:    source,
			Route:     route,
			UserAgent: c.Request().UserAgent(),
		})
	}
	return OK(c, nil)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

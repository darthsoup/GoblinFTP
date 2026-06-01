// Package sentry wraps github.com/getsentry/sentry-go for use in GoblinFTP.
// All functions are safe to call even if Sentry was never initialised.
package sentry

import (
	"fmt"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/labstack/echo/v4"
)

// Init initialises Sentry. If dsn is empty, it is a no-op.
func Init(dsn, environment, release string, sampleRate float64) error {
	if dsn == "" {
		return nil
	}
	if sampleRate == 0 {
		sampleRate = 1.0
	}
	return sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      environment,
		Release:          release,
		TracesSampleRate: sampleRate,
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			// Scrub PII: clear user context so usernames/hostnames are not sent.
			event.User = sentry.User{}
			return event
		},
	})
}

// Middleware returns an Echo middleware that captures panics and 5xx responses.
// If Sentry was not initialised the middleware is a pass-through.
func Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			hub := sentry.CurrentHub()
			if hub.Client() == nil {
				return next(c)
			}

			hub = hub.Clone()
			hub.Scope().SetRequest(c.Request())

			defer func() {
				if r := recover(); r != nil {
					hub.RecoverWithContext(c.Request().Context(), r)
					hub.Flush(2 * time.Second)
					c.Error(fmt.Errorf("internal server error: %v", r))
				}
			}()

			err := next(c)
			if err != nil {
				c.Error(err)
			}
			if c.Response().Status >= http.StatusInternalServerError {
				hub.CaptureMessage(fmt.Sprintf("%s %s → %d", c.Request().Method, c.Request().URL.Path, c.Response().Status))
				hub.Flush(2 * time.Second)
			}
			return nil
		}
	}
}

// Flush waits up to 2 s for buffered Sentry events to be sent.
func Flush() {
	sentry.Flush(2 * time.Second)
}

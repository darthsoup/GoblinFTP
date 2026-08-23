// Package sentry wraps github.com/getsentry/sentry-go for use in GoblinFTP.
// All functions are safe to call even if Sentry was never initialized.
package sentry

import (
	"fmt"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/labstack/echo/v4"
)

// Init initializes Sentry. If dsn is empty, it is a no-op.
//
// Middleware attaches the whole *http.Request, so the ?sso= token (which
// decrypts to a plaintext FTP password), the ?token= download token and the
// session cookie all pass through sentry-go's scrubber. That scrubber matches
// on the KEY NAME against a built-in denylist ("sso", "token", "session",
// "csrf", "auth", ...) - the value is never inspected. GoblinFTP is safe only
// because its parameter and cookie names happen to land on that list: rename
// ?sso= to ?ticket= and the token ships to Sentry in full.
// TestRequestSecretsAreFiltered pins this, so such a rename fails the build.
// SendDefaultPII is stated explicitly rather than left to the zero value so
// the choice is visible; it is not what protects these particular values.
func Init(dsn, environment, release string, sampleRate float64) error {
	if dsn == "" {
		return nil
	}
	return sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      environment,
		Release:          release,
		SendDefaultPII:   false,
		TracesSampleRate: sampleRate,
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			// Scrub PII: clear user context so usernames/hostnames are not sent.
			event.User = sentry.User{}
			return event
		},
	})
}

// Middleware returns an Echo middleware that captures panics and 5xx responses.
// If Sentry was not initialized the middleware is a pass-through.
//
// Events are queued, never flushed here: a per-request Flush held the handler
// goroutine for up to 2s on a network round trip, so anything that reliably
// produced a 5xx became a cheap way to tie up workers. main's deferred Flush
// drains the queue at shutdown.
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
					c.Error(fmt.Errorf("internal server error: %v", r))
				}
			}()

			err := next(c)
			if err != nil {
				c.Error(err)
			}
			if c.Response().Status >= http.StatusInternalServerError {
				hub.CaptureMessage(fmt.Sprintf("%s %s → %d", c.Request().Method, c.Request().URL.Path, c.Response().Status))
			}
			return nil
		}
	}
}

// Flush waits up to 2 s for buffered Sentry events to be sent.
func Flush() {
	sentry.Flush(2 * time.Second)
}

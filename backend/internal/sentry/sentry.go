// Package sentry wraps github.com/getsentry/sentry-go for use in GoblinFTP.
// All functions are safe to call even if Sentry was never initialized.
package sentry

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/labstack/echo/v4"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
)

// Options configures Init. An empty DSN disables reporting entirely.
type Options struct {
	DSN              string
	Environment      string
	Release          string
	TracesSampleRate float64
	ErrorSampleRate  float64
	// SendSessionContext allows the middleware to attach the FTP username, the
	// remote host, and a session prefix. Off by default: those identify a real
	// end customer to a third-party service.
	SendSessionContext bool
}

// Init initializes Sentry; an empty DSN is a no-op. Middleware attaches the whole
// request: sentry-go scrubs by KEY NAME, so renaming ?sso= would ship the password.
func Init(o Options) error {
	if o.DSN == "" {
		return nil
	}
	// sentry-go rewrites SampleRate 0 to 1.0 (client.go), so "sample nothing"
	// cannot be expressed through the option and is enforced in BeforeSend.
	dropAll := o.ErrorSampleRate <= 0
	sendSession := o.SendSessionContext
	return sentry.Init(sentry.ClientOptions{
		Dsn:              o.DSN,
		Environment:      o.Environment,
		Release:          o.Release,
		SendDefaultPII:   false,
		SampleRate:       o.ErrorSampleRate,
		EnableTracing:    o.TracesSampleRate > 0,
		TracesSampleRate: o.TracesSampleRate,
		AttachStacktrace: true,
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			if dropAll {
				return nil
			}
			// Last line of defense, not the only one: the middleware already
			// skips User unless opted in, so a future caller cannot leak it.
			if !sendSession {
				event.User = sentry.User{}
			}
			return event
		},
	})
}

// SessionInfo is the per-request identity Middleware attaches when the operator
// set SendSessionContext. IDPrefix is the 8-char session prefix, never the full ID.
type SessionInfo struct {
	IDPrefix string
	Username string
	Host     string
	Protocol string
}

// MiddlewareConfig keeps this package independent of internal/api: the hooks read
// the echo context keys that package owns.
type MiddlewareConfig struct {
	// LoggedError returns the GFTPError a handler committed through Fail, or nil.
	LoggedError func(echo.Context) *gftperrors.GFTPError
	// Session returns the connected session's identity, or the zero value.
	Session func(echo.Context) SessionInfo
	// CaptureRemoteErrors also reports faults on the far side of the connection.
	CaptureRemoteErrors bool
	SendSessionContext  bool
}

// remoteFaults are conditions on the far side of the connection: the customer's
// server, its TLS, its disk, or the network between. They are business events for
// the access log and Prometheus, so they stay out of Sentry unless asked for.
var remoteFaults = map[gftperrors.Code]bool{
	gftperrors.ErrConnectionFailed:     true,
	gftperrors.ErrConnectionLost:       true,
	gftperrors.ErrConnectionTimeout:    true,
	gftperrors.ErrTLSFailed:            true,
	gftperrors.ErrQuotaExceeded:        true,
	gftperrors.ErrStorageUnavailable:   true,
	gftperrors.ErrListFailed:           true,
	gftperrors.ErrOperationFailed:      true,
	gftperrors.ErrDataConnectionFailed: true,
	gftperrors.ErrTransferIncomplete:   true,
}

// eventLevel decides whether a finished request becomes an event and how loud it
// is. A host key change is reported whatever the setting: it can mean a MITM.
func eventLevel(status int, code gftperrors.Code, captureRemote bool) (sentry.Level, bool) {
	switch {
	case code == gftperrors.ErrHostKeyMismatch:
		return sentry.LevelWarning, true
	case status < http.StatusInternalServerError:
		return "", false
	case remoteFaults[code]:
		return sentry.LevelWarning, captureRemote
	default:
		return sentry.LevelError, true
	}
}

// routeOf returns the route template, which is what events group by. The concrete
// path would open one Sentry issue per tenant, per file, per upload ID.
func routeOf(c echo.Context) string {
	if p := c.Path(); p != "" {
		return p
	}
	return "unmatched"
}

// Middleware reports failed requests and provides a request-scoped hub. It must sit
// inside RequestID (the event carries the ID) and outside requestLogger, which is
// what commits the status this reads.
func Middleware(cfg MiddlewareConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			hub := sentry.CurrentHub()
			if hub.Client() == nil {
				return next(c)
			}

			hub = hub.Clone()
			req := c.Request()
			ctx := sentry.SetHubOnContext(req.Context(), hub)
			hub.Scope().SetRequest(req)

			var tx *sentry.Span
			if hub.Client().Options().EnableTracing {
				tx = sentry.StartTransaction(ctx, req.Method+" "+routeOf(c),
					sentry.ContinueTrace(hub,
						req.Header.Get(sentry.SentryTraceHeader),
						req.Header.Get(sentry.SentryBaggageHeader)),
					sentry.WithOpName("http.server"),
					sentry.WithTransactionSource(sentry.SourceRoute),
					sentry.WithSpanOrigin(sentry.SpanOriginEcho),
				)
				tx.SetData("http.request.method", req.Method)
				ctx = tx.Context()
			}
			c.SetRequest(req.WithContext(ctx))

			err := next(c)
			if err != nil {
				c.Error(err)
			}

			status := c.Response().Status
			if tx != nil {
				// The route template is only known after routing, so name the
				// transaction here rather than at StartTransaction.
				tx.Name = req.Method + " " + routeOf(c)
				tx.Status = sentry.HTTPtoSpanStatus(status)
				tx.SetData("http.response.status_code", status)
				tx.Finish()
			}

			report(c, hub, cfg, status)
			return nil
		}
	}
}

// report builds and sends the event for a finished request.
func report(c echo.Context, hub *sentry.Hub, cfg MiddlewareConfig, status int) {
	var ge *gftperrors.GFTPError
	if cfg.LoggedError != nil {
		ge = cfg.LoggedError(c)
	}
	code := ge.Code()

	level, ok := eventLevel(status, code, cfg.CaptureRemoteErrors)
	if !ok {
		return
	}

	route := routeOf(c)
	method := c.Request().Method
	reason := string(code)
	if reason == "" {
		reason = fmt.Sprintf("HTTP %d", status)
	}

	event := sentry.NewEvent()
	event.Level = level
	event.Message = method + " " + route + ": " + reason
	// Explicit fingerprint: without it Sentry groups messages by their text, and
	// any interpolated value would fan one defect out into many issues.
	event.Fingerprint = []string{method, route, reason}
	event.Tags = tagsFor(c, cfg, status, code)
	if ge != nil {
		event.Contexts["gftp"] = sentry.Context{"error_message": ge.Error()}
		// The cause is the raw server or network text classify() hid from the
		// client. It can name the remote host, which is why it is documented.
		if cause := ge.Unwrap(); cause != nil && cause.Error() != ge.Error() {
			event.Exception = []sentry.Exception{{Type: reason, Value: cause.Error()}}
		}
	}
	if cfg.SendSessionContext && cfg.Session != nil {
		if s := cfg.Session(c); s.Username != "" || s.IDPrefix != "" {
			event.User = sentry.User{ID: s.IDPrefix, Username: s.Username}
		}
	}

	hub.CaptureEvent(event)
}

func tagsFor(c echo.Context, cfg MiddlewareConfig, status int, code gftperrors.Code) map[string]string {
	tags := map[string]string{
		"route":       routeOf(c),
		"http.method": c.Request().Method,
		"http.status": fmt.Sprintf("%d", status),
		// The one field that ties a Sentry issue back to its access-log line.
		"request_id": c.Response().Header().Get(echo.HeaderXRequestID),
	}
	if code != "" {
		tags["error_code"] = string(code)
	}
	if code == gftperrors.ErrHostKeyMismatch {
		tags["security"] = "host_key_mismatch"
	}
	if cfg.Session != nil {
		s := cfg.Session(c)
		if s.Protocol != "" {
			tags["protocol"] = s.Protocol
		}
		if cfg.SendSessionContext {
			if s.Host != "" {
				tags["remote_host"] = s.Host
			}
			if s.IDPrefix != "" {
				tags["session"] = s.IDPrefix
			}
		}
	}
	return tags
}

// CapturePanic reports a panic that echo's Recover middleware already absorbed.
// Recover calls c.Error and swallows the panic, so no outer defer can ever see it.
func CapturePanic(c echo.Context, err error, stack []byte) {
	hub := sentry.GetHubFromContext(c.Request().Context())
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	if hub.Client() == nil {
		return
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelFatal)
		scope.SetTag("route", routeOf(c))
		scope.SetTag("request_id", c.Response().Header().Get(echo.HeaderXRequestID))
		// sentry-go rebuilds the stack from the live goroutine, which by now is
		// past the panic; echo captured the real frames before unwinding.
		scope.SetContext("panic", sentry.Context{"stack": string(stack)})
		hub.RecoverWithContext(c.Request().Context(), err)
	})
}

// FrontendError is one browser-side error relayed through POST /api/log/frontend.
type FrontendError struct {
	Kind      string
	Message   string
	Stack     string
	Source    string
	Route     string
	UserAgent string
}

// CaptureFrontend reports a browser error the SPA forwarded. The SPA reports
// directly when it has its own DSN, so this is the relay for when it does not.
func CaptureFrontend(e FrontendError) {
	hub := sentry.CurrentHub()
	if hub.Client() == nil {
		return
	}
	event := sentry.NewEvent()
	event.Level = sentry.LevelError
	event.Message = e.Message
	event.Fingerprint = []string{"frontend", e.Kind, e.Message, e.Source}
	event.Tags = map[string]string{
		"source":        "frontend",
		"frontend.kind": e.Kind,
	}
	if e.Route != "" {
		event.Tags["frontend.route"] = e.Route
	}
	event.Contexts["browser"] = sentry.Context{
		"stack":      e.Stack,
		"origin":     e.Source,
		"user_agent": e.UserAgent,
	}
	hub.CaptureEvent(event)
}

// Recover reports a panic in a background goroutine and re-panics, so the process
// still dies loudly instead of a goroutine vanishing with no event. Use as
// `defer sentry.Recover()` at the top of every goroutine that outlives a request.
func Recover() {
	r := recover()
	if r == nil {
		return
	}
	if hub := sentry.CurrentHub(); hub.Client() != nil {
		hub.RecoverWithContext(context.Background(), r)
		sentry.Flush(2 * time.Second)
	}
	panic(r)
}

// Flush waits up to 2 s for buffered Sentry events to be sent.
func Flush() {
	sentry.Flush(2 * time.Second)
}

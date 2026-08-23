package sentry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
)

// captureTransport collects events instead of shipping them, so the middleware
// can be asserted end to end without a network.
type captureTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *captureTransport) Configure(sentry.ClientOptions)        {}
func (t *captureTransport) Flush(time.Duration) bool              { return true }
func (t *captureTransport) FlushWithContext(context.Context) bool { return true }
func (t *captureTransport) Close()                                {}
func (t *captureTransport) SendEvent(event *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func (t *captureTransport) all() []*sentry.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*sentry.Event(nil), t.events...)
}

// newCapturingHub points the global hub at a capture transport and restores the
// previous hub when the test ends.
func newCapturingHub(t *testing.T, opts sentry.ClientOptions) *captureTransport {
	t.Helper()
	tr := &captureTransport{}
	opts.Dsn = testDSN
	opts.Transport = tr
	client, err := sentry.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	prev := sentry.CurrentHub()
	sentry.SetHubOnContext(context.Background(), prev)
	hub := sentry.NewHub(client, sentry.NewScope())
	old := sentry.CurrentHub().Client()
	sentry.CurrentHub().BindClient(client)
	t.Cleanup(func() { sentry.CurrentHub().BindClient(old) })
	_ = hub
	return tr
}

// newTestEcho mirrors the production middleware order from api.Register: the
// assertions below are worthless if this drifts from the real chain.
func newTestEcho(cfg MiddlewareConfig, handler echo.HandlerFunc) *echo.Echo {
	e := echo.New()
	e.Use(middleware.RequestID())
	e.Use(Middleware(cfg))
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			CapturePanic(c, err, stack)
			return err
		},
	}))
	e.GET("/api/files/:id", handler)
	return e
}

func failWith(ge *gftperrors.GFTPError) MiddlewareConfig {
	return MiddlewareConfig{
		LoggedError: func(echo.Context) *gftperrors.GFTPError { return ge },
	}
}

func do(t *testing.T, e *echo.Echo, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// TestPanicIsReportedWithStack is the regression test for the ordering bug: echo's
// Recover calls c.Error and returns nil, so a deferred recover in the outer
// middleware never fires and panics reached Sentry as a bare message.
func TestPanicIsReportedWithStack(t *testing.T) {
	tr := newCapturingHub(t, sentry.ClientOptions{})
	e := newTestEcho(failWith(nil), func(echo.Context) error { panic("boom") })

	do(t, e, "/api/files/7")

	events := tr.all()
	if len(events) == 0 {
		t.Fatal("no event captured for a panicking handler")
	}
	found := false
	for _, ev := range events {
		if ev.Level != sentry.LevelFatal || len(ev.Exception) == 0 {
			continue
		}
		found = true
		ex := ev.Exception[0]
		if ex.Stacktrace == nil || len(ex.Stacktrace.Frames) == 0 {
			t.Error("panic event carries no stack trace")
		}
		if !strings.Contains(ex.Value, "boom") {
			t.Errorf("panic value lost: %+v", ex)
		}
		if _, ok := ev.Contexts["panic"]; !ok {
			t.Error("echo's pre-unwind stack was not attached")
		}
	}
	if !found {
		t.Fatalf("no fatal event with an exception; got %d event(s)", len(events))
	}
}

// TestEventCarriesRequestIDAndCode pins the fields that make an event actionable:
// without request_id there is no way back to the access-log line.
func TestEventCarriesRequestIDAndCode(t *testing.T) {
	tr := newCapturingHub(t, sentry.ClientOptions{})
	ge := gftperrors.New(gftperrors.ErrInternal, "could not create session").
		WithCause(context.DeadlineExceeded)
	e := newTestEcho(failWith(ge), func(c echo.Context) error {
		return c.String(http.StatusInternalServerError, "fail")
	})

	rec := do(t, e, "/api/files/7")

	events := tr.all()
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	ev := events[0]
	if got := ev.Tags["request_id"]; got == "" || got != rec.Header().Get(echo.HeaderXRequestID) {
		t.Errorf("request_id tag = %q, response header = %q", got, rec.Header().Get(echo.HeaderXRequestID))
	}
	if got := ev.Tags["error_code"]; got != string(gftperrors.ErrInternal) {
		t.Errorf("error_code tag = %q", got)
	}
	if len(ev.Exception) == 0 || ev.Exception[0].Value != context.DeadlineExceeded.Error() {
		t.Errorf("cause was not attached: %+v", ev.Exception)
	}
}

// TestGroupingUsesRouteTemplate: the concrete path would open one Sentry issue per
// file, per tenant, per upload ID.
func TestGroupingUsesRouteTemplate(t *testing.T) {
	tr := newCapturingHub(t, sentry.ClientOptions{})
	ge := gftperrors.New(gftperrors.ErrInternal, "boom")
	e := newTestEcho(failWith(ge), func(c echo.Context) error {
		return c.String(http.StatusInternalServerError, "fail")
	})

	do(t, e, "/api/files/alpha")
	do(t, e, "/api/files/beta")

	events := tr.all()
	if len(events) != 2 {
		t.Fatalf("want 2 events, got %d", len(events))
	}
	if a, b := events[0].Fingerprint, events[1].Fingerprint; strings.Join(a, "|") != strings.Join(b, "|") {
		t.Errorf("two requests to one route produced different fingerprints: %v vs %v", a, b)
	}
	for _, ev := range events {
		if strings.Contains(ev.Message, "alpha") || strings.Contains(ev.Message, "beta") {
			t.Errorf("concrete path leaked into the grouping message: %q", ev.Message)
		}
	}
}

// TestRemoteFaultsAreNotDefects: a customer typing the wrong hostname is a 500,
// and paging a developer for it is what makes a Sentry project unreadable.
func TestRemoteFaultsAreNotDefects(t *testing.T) {
	for _, code := range []gftperrors.Code{
		gftperrors.ErrConnectionFailed,
		gftperrors.ErrConnectionLost,
		gftperrors.ErrConnectionTimeout,
		gftperrors.ErrTLSFailed,
		gftperrors.ErrQuotaExceeded,
	} {
		t.Run(string(code), func(t *testing.T) {
			tr := newCapturingHub(t, sentry.ClientOptions{})
			ge := gftperrors.New(code, "remote said no")
			e := newTestEcho(failWith(ge), func(c echo.Context) error {
				return c.String(http.StatusBadGateway, "fail")
			})
			do(t, e, "/api/files/7")
			if n := len(tr.all()); n != 0 {
				t.Errorf("remote fault %s produced %d event(s) with capture off", code, n)
			}
		})
	}
}

// TestRemoteFaultsCapturedWhenAsked is the opt-in half: an admin chasing a
// customer connection problem turns them on.
func TestRemoteFaultsCapturedWhenAsked(t *testing.T) {
	tr := newCapturingHub(t, sentry.ClientOptions{})
	cfg := failWith(gftperrors.New(gftperrors.ErrConnectionFailed, "could not connect to server"))
	cfg.CaptureRemoteErrors = true
	e := newTestEcho(cfg, func(c echo.Context) error {
		return c.String(http.StatusInternalServerError, "fail")
	})

	do(t, e, "/api/files/7")

	events := tr.all()
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Level != sentry.LevelWarning {
		t.Errorf("remote fault level = %v, want warning", events[0].Level)
	}
}

// TestHostKeyMismatchAlwaysReported: it can mean a man in the middle, so it is
// not silenced by the remote-fault default.
func TestHostKeyMismatchAlwaysReported(t *testing.T) {
	tr := newCapturingHub(t, sentry.ClientOptions{})
	ge := gftperrors.New(gftperrors.ErrHostKeyMismatch, "host key changed")
	e := newTestEcho(failWith(ge), func(c echo.Context) error {
		return c.String(http.StatusBadGateway, "fail")
	})

	do(t, e, "/api/files/7")

	events := tr.all()
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Tags["security"] != "host_key_mismatch" {
		t.Errorf("security tag missing: %v", events[0].Tags)
	}
}

func TestSuccessAndClientErrorsAreSilent(t *testing.T) {
	tr := newCapturingHub(t, sentry.ClientOptions{})
	cfg := failWith(gftperrors.New(gftperrors.ErrAuthFailed, "authentication failed"))
	e := newTestEcho(cfg, func(c echo.Context) error {
		return c.String(http.StatusUnauthorized, "nope")
	})

	do(t, e, "/api/files/7")

	if n := len(tr.all()); n != 0 {
		t.Errorf("a 401 produced %d event(s)", n)
	}
}

// TestSessionContextIsOptIn pins the privacy default: the FTP username and remote
// host identify a real end customer, so they never leave without consent.
func TestSessionContextIsOptIn(t *testing.T) {
	session := func(echo.Context) SessionInfo {
		return SessionInfo{IDPrefix: "abcd1234", Username: "alice", Host: "ftp.customer.example:21", Protocol: "sftp"}
	}

	for _, tc := range []struct {
		name   string
		optIn  bool
		wantPI bool
	}{
		{"default", false, false},
		{"opted in", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := newCapturingHub(t, sentry.ClientOptions{SendDefaultPII: false})
			cfg := failWith(gftperrors.New(gftperrors.ErrInternal, "boom"))
			cfg.Session = session
			cfg.SendSessionContext = tc.optIn
			e := newTestEcho(cfg, func(c echo.Context) error {
				return c.String(http.StatusInternalServerError, "fail")
			})

			do(t, e, "/api/files/7")

			events := tr.all()
			if len(events) != 1 {
				t.Fatalf("want 1 event, got %d", len(events))
			}
			ev := events[0]
			// The protocol is not an identifier, so it is always useful and always sent.
			if ev.Tags["protocol"] != "sftp" {
				t.Errorf("protocol tag = %q, want sftp", ev.Tags["protocol"])
			}
			gotPI := ev.User.Username != "" || ev.Tags["remote_host"] != "" || ev.Tags["session"] != ""
			if gotPI != tc.wantPI {
				t.Errorf("session context present = %v, want %v (user=%+v tags=%v)", gotPI, tc.wantPI, ev.User, ev.Tags)
			}
		})
	}
}

// TestErrorSampleRateZeroDropsEverything: sentry-go rewrites SampleRate 0 to 1.0,
// so without the BeforeSend guard this setting would send every event instead of none.
func TestErrorSampleRateZeroDropsEverything(t *testing.T) {
	if err := Init(Options{DSN: testDSN, ErrorSampleRate: 0}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = Init(Options{DSN: testDSN, ErrorSampleRate: 1}) })

	tr := &captureTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:        testDSN,
		Transport:  tr,
		BeforeSend: sentry.CurrentHub().Client().Options().BeforeSend,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	old := sentry.CurrentHub().Client()
	sentry.CurrentHub().BindClient(client)
	t.Cleanup(func() { sentry.CurrentHub().BindClient(old) })

	sentry.CurrentHub().CaptureMessage("should be dropped")
	if n := len(tr.all()); n != 0 {
		t.Errorf("ErrorSampleRate 0 sent %d event(s), want 0", n)
	}
}

// TestCaptureFrontendIsTaggedAsBrowser: relayed browser errors must be separable
// from backend defects, or the two audiences share one noisy stream.
func TestCaptureFrontendIsTaggedAsBrowser(t *testing.T) {
	tr := newCapturingHub(t, sentry.ClientOptions{})

	CaptureFrontend(FrontendError{
		Kind: "vue", Message: "Cannot read properties of undefined",
		Stack: "at render (app.js:1:1)", Source: "app.js:1:1", Route: "/edit",
	})

	events := tr.all()
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Tags["source"] != "frontend" || ev.Tags["frontend.kind"] != "vue" {
		t.Errorf("tags = %v", ev.Tags)
	}
	if ev.Contexts["browser"]["stack"] != "at render (app.js:1:1)" {
		t.Errorf("browser stack lost: %v", ev.Contexts["browser"])
	}
}

package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func getHeaders(t *testing.T, cfg *config.Config, path string) http.Header {
	t.Helper()
	app, _, _ := newTestApp(t, cfg)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec.Result().Header
}

// Unconfigured must mean denied. GoblinFTP previously shipped no framing
// restriction at all, so this is the assertion that pins the new default.
func TestCSPDeniesFramingByDefault(t *testing.T) {
	for _, path := range []string{"/healthz", "/api/system/vars"} {
		h := getHeaders(t, defaultTestConfig(), path)
		assert.Contains(t, h.Get("Content-Security-Policy"), "frame-ancestors 'none'", path)
		assert.Equal(t, "DENY", h.Get("X-Frame-Options"), path)
	}
}

func TestCSPEmitsAllowlistWhenConfigured(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.FrameAncestors = []string{"https://panel.example.com", "https://eu.panel.example.com"}

	h := getHeaders(t, cfg, "/api/system/vars")

	assert.Contains(t, h.Get("Content-Security-Policy"),
		"frame-ancestors https://panel.example.com https://eu.panel.example.com")
	// Pins the XFO decision: it cannot express an allowlist, and an engine that
	// preferred it would silently override frame-ancestors and break framing.
	assert.Empty(t, h.Values("X-Frame-Options"),
		"X-Frame-Options must be absent when an allowlist is configured")
}

// A second Content-Security-Policy header is enforced as the intersection of
// both policies - correct, but near-impossible to debug from a console.
func TestCSPIsASingleHeaderWithBaseDirectivesIntact(t *testing.T) {
	h := getHeaders(t, defaultTestConfig(), "/api/system/vars")

	require.Len(t, h.Values("Content-Security-Policy"), 1)
	csp := h.Get("Content-Security-Policy")
	for _, directive := range []string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
	} {
		assert.Contains(t, csp, directive)
	}
}

// THE load-bearing invariant of the embed design. Under SameSite=None the CSRF
// token is the only CSRF defense left, and it holds solely because a
// cross-origin page cannot read GET /api/auth/status - which is true only while
// no Access-Control-Allow-Origin is ever emitted. If someone adds CORS
// middleware, this test must fail loudly.
func TestNoCORSHeadersEver(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.FrameAncestors = []string{"https://panel.example.com"}
	app, _, _ := newTestApp(t, cfg)

	for _, path := range []string{"/api/auth/status", "/api/files", "/api/system/vars"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		h := rec.Result().Header
		assert.Empty(t, h.Values("Access-Control-Allow-Origin"), path)
		assert.Empty(t, h.Values("Access-Control-Allow-Credentials"), path)
	}
}

// Defense in depth for the CSRF-exempt connect endpoint. Not closing a live
// hole (echo's binder ignores untagged form fields, so a cross-site simple
// request binds nothing), but the guard should not depend on that detail.
func TestConnectRejectsCrossSiteFetch(t *testing.T) {
	app, _, _ := newTestApp(t, defaultTestConfig(), staticDialOption())

	body := `{"protocol":"ftp","host":"h","port":21,"username":"u","password":"p"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_FORBIDDEN")
}

func TestConnectAllowsSameSiteAndMissingFetchMetadata(t *testing.T) {
	for _, site := range []string{"same-origin", "same-site", "none", ""} {
		app, _, _ := newTestApp(t, defaultTestConfig(), staticDialOption())
		body := `{"protocol":"ftp","host":"h","port":21,"username":"u","password":"p"}`
		req := httptest.NewRequest(http.MethodPost, "/api/auth/connect", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code, "Sec-Fetch-Site: %q", site)
	}
}

func staticDialOption() api.HandlerOption {
	return api.WithDial(staticDial(&testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
	}))
}

package api_test

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/config"
)

func TestSessionCookieSchemeTrustsOnlyConfiguredPeers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		peer   string
		tls    bool
		secure bool
	}{
		{"trusted Caddy", "127.0.0.1:1234", false, true},
		{"untrusted peer", "198.51.100.20:1234", false, false},
		{"direct TLS", "198.51.100.20:1234", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultTestConfig()
			cfg.Settings.Access.TrustedProxies = []string{"127.0.0.1/32"}
			app, _, _ := newTestApp(t, cfg, staticDialOption())
			req := httptest.NewRequest(http.MethodPost, "/api/auth/connect",
				strings.NewReader(`{"protocol":"ftp","host":"h","port":21,"username":"u","password":"p"}`))
			req.RemoteAddr = tc.peer
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-Proto", "https")
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tc.secure, strings.Contains(rawSetCookie(t, rec), "; Secure"))
		})
	}
}

// rawSetCookie returns the Set-Cookie header verbatim: net/http's parser drops
// attributes it does not model, and Partitioned would silently vanish.
func rawSetCookie(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	for _, v := range rec.Result().Header.Values("Set-Cookie") {
		if strings.HasPrefix(v, "gftp_session=") || strings.HasPrefix(v, "__Host-gftp_session=") {
			return v
		}
	}
	t.Fatalf("no gftp_session cookie in %v", rec.Result().Header.Values("Set-Cookie"))
	return ""
}

func connectRec(t *testing.T, cfg *config.Config) *httptest.ResponseRecorder {
	t.Helper()
	app, _, _ := newTestApp(t, cfg, staticDialOption())
	body := `{"protocol":"ftp","host":"h","port":21,"username":"u","password":"p"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec
}

func TestSessionCookieLaxByDefault(t *testing.T) {
	raw := rawSetCookie(t, connectRec(t, defaultTestConfig()))

	assert.Contains(t, raw, "SameSite=Lax")
	assert.NotContains(t, raw, "SameSite=None")
	assert.NotContains(t, raw, "Partitioned")
	assert.Contains(t, raw, "HttpOnly")
}

// The plain-HTTP request is the point: behind an external TLS terminator a Secure
// derived from c.Scheme() would be false, so the browser drops the cookie.
func TestSessionCookieForcesSecureWhenEmbedding(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.FrameAncestors = []string{"https://panel.example.com"}

	raw := rawSetCookie(t, connectRec(t, cfg))

	assert.Contains(t, raw, "SameSite=None")
	assert.Contains(t, raw, "Secure")
	assert.Contains(t, raw, "Partitioned")
}

// A browser only replaces or deletes a cookie when every attribute matches, so
// a clear that diverged from the set would silently leave the session in place.
func TestDisconnectClearsCookieWithMatchingAttributes(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.FrameAncestors = []string{"https://panel.example.com"}
	app, _, _ := newTestApp(t, cfg, staticDialOption())
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/disconnect", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	raw := rawSetCookie(t, rec)
	assert.Contains(t, raw, "SameSite=None")
	assert.Contains(t, raw, "Secure")
	assert.Contains(t, raw, "Partitioned")
	assert.Contains(t, raw, "Max-Age=0")
}

// HTTPS must ignore legacy cookies even when they accompany a valid prefixed cookie.
func TestHTTPSIgnoresLegacyCookieBesidePrefixedSession(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.FrameAncestors = []string{"https://panel.example.com"}
	app, _, _ := newTestApp(t, cfg, staticDialOption())
	sess := connectAndGetSession(t, app)

	for _, order := range []string{"stale first", "stale last"} {
		t.Run(order, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
			if order == "stale first" {
				req.AddCookie(&http.Cookie{Name: "gftp_session", Value: "stale-from-before-the-upgrade"})
			}
			for _, c := range sess.cookies {
				req.AddCookie(c)
			}
			if order == "stale last" {
				req.AddCookie(&http.Cookie{Name: "gftp_session", Value: "stale-from-before-the-upgrade"})
			}
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			assert.NotEqual(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), "ERR_SESSION_NOT_FOUND")
		})
	}
}

// Stale or duplicate cookies never establish a session.
func TestOnlyStaleSessionCookiesStillRejected(t *testing.T) {
	app, _, _ := newTestApp(t, defaultTestConfig(), staticDialOption())

	req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
	req.AddCookie(&http.Cookie{Name: "gftp_session", Value: "nope"})
	req.AddCookie(&http.Cookie{Name: "gftp_session", Value: "also-nope"})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_SESSION_NOT_FOUND")
}

package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/config"
)

// rawSetCookie returns the Set-Cookie header verbatim: net/http's parser drops
// attributes it does not model, and Partitioned would silently vanish.
func rawSetCookie(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	for _, v := range rec.Result().Header.Values("Set-Cookie") {
		if strings.HasPrefix(v, "gftp_session=") {
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

// Chrome keeps a Partitioned cookie as a distinct entry, so enabling framing on a
// live deployment sends both. Reading only the first would 401 every request.
func TestStaleDuplicateSessionCookieIsIgnored(t *testing.T) {
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

// The multi-cookie tolerance must not become "accept anything": only unresolvable
// session cookies is still a rejection.
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

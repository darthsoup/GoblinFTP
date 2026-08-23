package api

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
)

// lookupSession resolves the request's session, trying every gftp_session cookie:
// a Partitioned and an unpartitioned one coexist in the jar, and the first may be stale.
func lookupSession(c echo.Context, store *auth.Store) (*auth.Session, bool) {
	for _, ck := range c.Request().Cookies() {
		if ck.Name != SessionCookieName {
			continue
		}
		if sess, ok := store.Get(ck.Value); ok {
			return sess, true
		}
	}
	return nil, false
}

// hasSessionCookie reports whether any gftp_session cookie was sent, resolvable
// or not. Distinguishes "not authenticated" from "session expired".
func hasSessionCookie(c echo.Context) bool {
	for _, ck := range c.Request().Cookies() {
		if ck.Name == SessionCookieName {
			return true
		}
	}
	return false
}

// sessionCookie builds the gftp_session cookie. Clearing (maxAge -1) must go
// through here too: a browser only drops a cookie when every attribute matches.
func sessionCookie(c echo.Context, cfg *config.Config, value string, maxAge int) *http.Cookie {
	ck := &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		// Secure when served over TLS. Behind an external terminator only
		// X-Forwarded-Proto signals that, believed for trusted proxies alone (proxy.go).
		Secure:   clientScheme(c, cfg) == "https",
		SameSite: http.SameSiteLaxMode,
	}

	if cfg.EmbeddingEnabled() {
		// A cross-site iframe needs SameSite=None, which browsers reject without
		// Secure. Forced, not derived: Caddy reports X-Forwarded-Proto: http here.
		ck.SameSite = http.SameSiteNoneMode
		ck.Secure = true
		// Partitioned (CHIPS) survives third-party-cookie restrictions and keys the
		// framed session to the embedding site, so no top-level tab shares it.
		ck.Partitioned = true
	}
	return ck
}

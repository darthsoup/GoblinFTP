package api

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
)

// lookupSession resolves the request's session, trying every gftp_session
// cookie rather than just the first.
//
// A browser can legitimately hold more than one cookie of the same name: a
// Partitioned cookie and an unpartitioned one are distinct entries, so enabling
// GFTP_FRAME_ANCESTORS on a deployment whose users already hold a SameSite=Lax
// session leaves both in the jar. They are sent together, and picking only the
// first would hand back the stale one and 401 every request until the user
// cleared their cookies by hand.
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

// hasSessionCookie reports whether any gftp_session cookie was sent, regardless
// of whether it still resolves. Distinguishes "not authenticated" from
// "session expired".
func hasSessionCookie(c echo.Context) bool {
	for _, ck := range c.Request().Cookies() {
		if ck.Name == SessionCookieName {
			return true
		}
	}
	return false
}

// sessionCookie builds the gftp_session cookie with the deployment's
// SameSite/Secure policy. Both setting (maxAge 0) and clearing (value "",
// maxAge -1) go through here on purpose: a browser only replaces or deletes a
// cookie when every attribute matches, so a clear that diverged from the set
// would silently no-op and leave the session cookie in place.
func sessionCookie(c echo.Context, cfg *config.Config, value string, maxAge int) *http.Cookie {
	ck := &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		// Secure when served over TLS. Behind an external terminator Caddy
		// listens on plain HTTP inside the container and forwards
		// X-Forwarded-Proto, so that header is the only signal - but it is
		// only believed when a proxy allowlist says so (see proxy.go). Without
		// one, plain-HTTP LAN deployments keep working.
		Secure:   clientScheme(c, cfg) == "https",
		SameSite: http.SameSiteLaxMode,
	}

	if cfg.EmbeddingEnabled() {
		// A cross-site iframe only receives the cookie with SameSite=None, which
		// browsers reject unless Secure is also set. Secure is forced here
		// rather than derived from c.Scheme(): behind an external TLS
		// terminator Caddy listens on :80 inside the container and forwards
		// X-Forwarded-Proto: http, so c.Scheme() reports "http" and the cookie
		// would be dropped with no diagnostic. Config rejects non-loopback
		// http:// ancestors for the same reason.
		//
		// Partitioned (CHIPS) keeps the cookie working under Chrome's
		// third-party-cookie restrictions. It also means the framed session is
		// keyed to the embedding site and is NOT shared with a top-level tab.
		ck.SameSite = http.SameSiteNoneMode
		ck.Secure = true
		ck.Partitioned = true
	}
	return ck
}

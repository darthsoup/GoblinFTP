package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
)

func requestCookieName(c echo.Context) string {
	if name, ok := c.Get("sessionCookieName").(string); ok {
		return name
	}
	return SessionCookieName
}

func lookupNamedSession(c echo.Context, store *auth.Store, name string) (*auth.Session, bool) {
	var value string
	count := 0
	for _, ck := range c.Request().Cookies() {
		if ck.Name == name {
			count++
			value = ck.Value
		}
	}
	if count != 1 {
		return nil, false
	}
	return store.Get(value)
}

func lookupSession(c echo.Context, store *auth.Store) (*auth.Session, bool) {
	return lookupNamedSession(c, store, requestCookieName(c))
}

func hasSessionCookie(c echo.Context) bool {
	for _, ck := range c.Request().Cookies() {
		if ck.Name == requestCookieName(c) {
			return true
		}
	}
	return false
}

func writeSessionCookie(c echo.Context, ck *http.Cookie) {
	c.SetCookie(ck)
	if !ck.Secure {
		return
	}
	// CHIPS and ordinary cookies occupy separate slots in the browser cookie jar.
	opposite := *ck //nolint:gosec // G124: copies the secure, HTTP-only cookie policy for deletion.
	opposite.Value = ""
	opposite.MaxAge = -1
	opposite.Partitioned = !ck.Partitioned
	if opposite.Partitioned {
		opposite.SameSite = http.SameSiteNoneMode
	}
	c.SetCookie(&opposite)
	if strings.HasPrefix(ck.Name, "__Host-") {
		legacy := *ck //nolint:gosec // G124: copies the secure, HTTP-only cookie policy for deletion.
		legacy.Name = strings.TrimPrefix(ck.Name, "__Host-")
		legacy.Value = ""
		legacy.MaxAge = -1
		c.SetCookie(&legacy)
		legacy.Partitioned = !legacy.Partitioned
		if legacy.Partitioned {
			legacy.SameSite = http.SameSiteNoneMode
		}
		c.SetCookie(&legacy)
	}
}

func setSessionCookie(c echo.Context, cfg *config.Config, value string, maxAge int) {
	writeSessionCookie(c, sessionCookie(c, cfg, value, maxAge))
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
	if ck.Secure {
		ck.Name = "__Host-gftp_session"
	}
	return ck
}

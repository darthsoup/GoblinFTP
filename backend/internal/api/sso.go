// backend/internal/api/sso.go
package api

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/sso"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

const ssoPendingKey = "sso_pending"

// tokenHash returns the hex-encoded SHA-256 hash of raw (for replay detection).
func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum)
}

// ssoReject logs an SSO login rejection and redirects the browser back to the
// SPA with an ?sso_error=<reason> marker (no sso= param, so Caddy serves the
// SPA rather than re-proxying to this handler). The redirect bypasses Fail's
// access-log stash, so the reason is logged explicitly here for ops visibility.
func (h *Handler) ssoReject(c echo.Context, reason string, cause error) error {
	attrs := []slog.Attr{slog.String("reason", reason)}
	if cause != nil {
		attrs = append(attrs, slog.String("cause", cause.Error()))
	}
	h.logger.LogAttrs(c.Request().Context(), slog.LevelWarn, "sso login rejected", attrs...)
	return c.Redirect(http.StatusFound, "/login?sso_error="+reason)
}

// SSOLogin handles GET /?sso=<token>.
// If no sso param: returns 200 placeholder (SPA serving will be added later).
// On any token rejection: redirect to /login?sso_error=<reason> so the SPA can
// show a message. On success: create session, redirect to /login.
func (h *Handler) SSOLogin(c echo.Context) error {
	raw := c.QueryParam("sso")
	if raw == "" {
		return c.String(http.StatusOK, "GoblinFTP")
	}

	if !h.cfg.SSOEnabled {
		return h.ssoReject(c, "disabled", nil)
	}

	payload, err := sso.Decrypt(raw, h.cfg.SSOSecret)
	if err != nil {
		if errors.Is(err, sso.ErrTokenExpired) {
			return h.ssoReject(c, "expired", nil)
		}
		return h.ssoReject(c, "invalid", err)
	}

	hash := tokenHash(raw)
	if h.ssoUsed.IsUsed(hash) {
		return h.ssoReject(c, "used", nil)
	}
	h.ssoUsed.Mark(hash, time.Unix(payload.Exp, 0))

	csrfToken, csrfErr := auth.GenerateCSRFToken()
	if csrfErr != nil {
		return h.ssoReject(c, "internal", csrfErr)
	}

	sess, sessErr := h.store.New()
	if sessErr != nil {
		return h.ssoReject(c, "internal", sessErr)
	}
	sess.Set(auth.CSRFSessionKey, csrfToken)
	sess.Set(ssoPendingKey, ConnectRequest{
		Protocol: payload.Type,
		Host:     payload.Host,
		Port:     payload.Port,
		Username: payload.Username,
		Password: payload.Password,
	})

	c.SetCookie(&http.Cookie{ //nolint:gosec // G124: Secure is set conditionally below — literal true would break plain-HTTP deployments
		Name:     SessionCookieName,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.Scheme() == "https",
		SameSite: http.SameSiteLaxMode,
	})

	// Land on the SPA login route, which finalizes the connection via
	// /api/auth/sso-connect (ssoAutoConnect) and then routes to the workspace.
	return c.Redirect(http.StatusFound, "/login")
}

// AuthStatus handles GET /api/auth/status.
// Public endpoint: no requireSession middleware. Manually reads session cookie.
// Returns {connected, ssoAutoConnect, csrfToken} and, when connected, the
// session's connection context (host, initialDirectory, capabilities) so the
// SPA can restore its state after a page reload.
// With ?ping=1 the FTP/SFTP connection is verified with a lightweight round
// trip; a dead connection is closed, removed from the session, and reported
// as connected=false.
func (h *Handler) AuthStatus(c echo.Context) error {
	type statusData struct {
		Connected        bool          `json:"connected"`
		SSOAutoConnect   bool          `json:"ssoAutoConnect"`
		CSRFToken        string        `json:"csrfToken"`
		Host             string        `json:"host,omitempty"`
		InitialDirectory string        `json:"initialDirectory,omitempty"`
		Capabilities     *Capabilities `json:"capabilities,omitempty"`
	}

	result := statusData{}

	cookie, err := c.Cookie(SessionCookieName)
	if err == nil {
		if sess, ok := h.store.Get(cookie.Value); ok {
			clientVal, hasClient := sess.Get("client")
			result.Connected = hasClient
			client, _ := clientVal.(transfer.Client)
			if hasClient && client != nil && c.QueryParam("ping") == "1" {
				// Only ping when no transfer is in flight: an active transfer is
				// itself proof the connection is alive, and a NOOP injected mid
				// data-stream would corrupt it. TryLock avoids blocking the
				// session-checker behind a long upload/download.
				if sess.TryLockTransfer() {
					pingErr := client.Ping()
					if pingErr != nil {
						_ = client.Close()
					}
					sess.UnlockTransfer()
					if pingErr != nil {
						sess.Delete("client")
						result.Connected = false
					}
				}
			}
			_, result.SSOAutoConnect = sess.Get(ssoPendingKey)
			result.CSRFToken = sess.GetString(auth.CSRFSessionKey)

			// Connection context for SPA state restoration after a reload.
			if result.Connected {
				result.Host = sess.GetString("host")
				result.InitialDirectory = sess.GetString("initialDir")
				disableChmodVal, _ := sess.Get("disableChmod")
				disableChmod, _ := disableChmodVal.(bool)
				result.Capabilities = &Capabilities{DisableChmod: disableChmod}
			}
		}
	}

	return OK(c, result)
}

// SSOConnect handles POST /api/auth/sso-connect.
// Requires valid session (enforced by requireSession middleware).
// Reads the pending SSO ConnectRequest from session, dials, and returns ConnectData.
func (h *Handler) SSOConnect(c echo.Context) error {
	sess := c.Get("session").(*auth.Session)

	pendingVal, ok := sess.Get(ssoPendingKey)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "no pending SSO connection"))
	}
	pending, ok := pendingVal.(ConnectRequest)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "no pending SSO connection"))
	}

	if gftperr := h.checkIPAllowlist(c); gftperr != nil {
		return Fail(c, gftperr)
	}

	addr := fmt.Sprintf("%s:%d", pending.Host, pending.Port)
	client, dialErr := h.dial(pending.Protocol, addr, pending.Username, pending.Password, pending.Passive)
	if dialErr != nil {
		if errors.Is(dialErr, transfer.ErrAuthFailed) {
			h.metrics.ConnectAttempts.WithLabelValues(pending.Protocol, "auth_failed").Inc()
			return Fail(c, gftperrors.New(gftperrors.ErrAuthFailed, "authentication failed").WithCause(dialErr))
		}
		h.metrics.ConnectAttempts.WithLabelValues(pending.Protocol, "failed").Inc()
		return Fail(c, gftperrors.New(gftperrors.ErrConnectionFailed, "could not connect to server").WithCause(dialErr))
	}

	initialDir, wdErr := client.WorkingDir()
	if wdErr != nil {
		_ = client.Close()
		return Fail(c, gftperrors.New(gftperrors.ErrConnectionFailed, "could not get working directory").WithCause(wdErr))
	}

	disableChmod := detectChmod(client, pending.Protocol, initialDir)

	sess.Set("client", client)
	sess.Set("initialDir", initialDir)
	sess.Set("disableChmod", disableChmod)
	// For access-log and metrics enrichment only — never the password.
	sess.Set("username", pending.Username)
	sess.Set("host", addr)
	sess.Set("protocol", pending.Protocol)
	sess.Delete(ssoPendingKey)

	h.metrics.ConnectAttempts.WithLabelValues(pending.Protocol, "success").Inc()

	csrfToken := sess.GetString(auth.CSRFSessionKey)

	return OK(c, ConnectData{
		Capabilities:     Capabilities{DisableChmod: disableChmod},
		InitialDirectory: initialDir,
		CSRFToken:        csrfToken,
	})
}

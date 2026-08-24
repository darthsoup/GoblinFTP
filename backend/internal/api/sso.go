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
	"github.com/darthsoup/goblinftp/internal/logging"
	"github.com/darthsoup/goblinftp/internal/sso"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

const ssoPendingKey = "sso_pending"

// tokenHash returns the hex-encoded SHA-256 hash of raw (for replay detection).
func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum)
}

// ssoReject redirects to the SPA with ?sso_error=<reason> (no sso= param, so
// Caddy serves the SPA). The redirect bypasses Fail's access-log stash, hence the log.
func (h *Handler) ssoReject(c echo.Context, reason string, cause error) error {
	attrs := []slog.Attr{slog.String("reason", reason)}
	if cause != nil {
		attrs = append(attrs, slog.String("cause", cause.Error()))
	}
	h.logger.LogAttrs(c.Request().Context(), slog.LevelWarn, "sso login rejected", attrs...)
	return c.Redirect(http.StatusFound, "/login?sso_error="+reason)
}

// SSOLogin handles GET /?sso=<token>. Any token rejection redirects to
// /login?sso_error=<reason> so the SPA can show a message.
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

	// The allowlist is the operator's policy, not necessarily the token minter's
	// (white-label), so it is enforced here rather than trusted from the payload.
	if !isAllowedType(payload.Type, h.cfg.Settings.Connection.AllowedTypes) {
		return h.ssoReject(c, "invalid", fmt.Errorf("connection type %q is not allowed", payload.Type))
	}
	if gftperr := h.checkHostLock(payload.Host, payload.Port); gftperr != nil {
		return h.ssoReject(c, "invalid", gftperr)
	}

	// Atomic: IsUsed-then-Mark let two concurrent requests with the same
	// one-time token both mint a session.
	if !h.ssoUsed.MarkIfUnused(tokenHash(raw), time.Unix(payload.Exp, 0)) {
		return h.ssoReject(c, "used", nil)
	}

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
		// sso.Payload carries no passive flag, so this defaulted to false and
		// every SSO FTP login dialed in active mode, failing behind NAT.
		Passive: h.cfg.Settings.Connection.PassiveMode,
	})
	// Carry the validated tenant on the session so it survives the pending to
	// connected transition. An unknown tenant is dropped silently, login still works.
	if tenant := sanitizeTenant(payload.Tenant); tenant != "" {
		sess.Set(tenantSessionKey, tenant)
	}

	// Set on the cross-site iframe navigation itself, so it must carry the embed
	// policy or the frame never establishes a session.
	c.SetCookie(sessionCookie(c, h.cfg, sess.ID, 0))

	// Land on the SPA login route, which finalizes the connection via
	// /api/auth/sso-connect (ssoAutoConnect) and then routes to the workspace.
	return c.Redirect(http.StatusFound, "/login")
}

// AuthStatus handles GET /api/auth/status. Public: it reads the session cookie
// itself. With ?ping=1 a dead connection is closed and reported as connected=false.
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

	if sess, ok := lookupSession(c, h.store); ok {
		clientVal, hasClient := sess.Get("client")
		result.Connected = hasClient
		client, _ := clientVal.(transfer.Client)
		if hasClient && client != nil && c.QueryParam("ping") == "1" {
			// Only ping when idle: a NOOP injected mid data-stream would corrupt it,
			// and TryLock avoids blocking the checker behind a long transfer.
			if sess.TryLockTransfer() {
				pingErr := client.Ping()
				if pingErr != nil {
					_ = client.Close()
				}
				sess.UnlockTransfer()
				if pingErr != nil {
					sess.Delete("client")
					result.Connected = false
					code, _ := classify(pingErr)
					attrs := []slog.Attr{slog.String("code", string(code))}
					attrs = append(attrs, logging.SafeLogAttrs(slog.String("cause", pingErr.Error()))...)
					h.logger.LogAttrs(c.Request().Context(), slog.LevelWarn,
						"connection dropped, session marked disconnected", attrs...)
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

	return OK(c, result)
}

// SSOConnect handles POST /api/auth/sso-connect: reads the pending SSO
// ConnectRequest from the session, dials, and returns ConnectData.
func (h *Handler) SSOConnect(c echo.Context) error {
	sess, ok := c.Get("session").(*auth.Session)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active session"))
	}

	pendingVal, ok := sess.Get(ssoPendingKey)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "no pending SSO connection"))
	}
	pending, ok := pendingVal.(ConnectRequest)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "no pending SSO connection"))
	}

	// Optional body: the SHA256 fingerprint the user agreed to trust when the
	// first attempt returned an unknown SFTP host key (empty on first attempt).
	var body struct {
		AcceptHostKey string `json:"acceptHostKey"`
	}
	if gerr := bindJSON(c, &body); gerr != nil {
		return Fail(c, gerr)
	}

	if gftperr := h.checkIPAllowlist(c); gftperr != nil {
		return Fail(c, gftperr)
	}

	addr := fmt.Sprintf("%s:%d", pending.Host, pending.Port)
	client, hostKey, dialErr := h.dial(DialRequest{
		Protocol:      pending.Protocol,
		Addr:          addr,
		Host:          pending.Host,
		User:          pending.Username,
		Pass:          pending.Password,
		Passive:       pending.Passive,
		AcceptHostKey: body.AcceptHostKey,
	})
	if hostKey != nil {
		// Keep the pending SSO request so the SPA can retry with the trusted key.
		h.metrics.ConnectAttempts.WithLabelValues(pending.Protocol, hostKeyLabel(hostKey)).Inc()
		noteHostKeyChange(c, h.logger, pending.Host, hostKey)
		return OK(c, ConnectData{HostKeyPrompt: hostKey})
	}
	if dialErr != nil {
		failure := classifyDial(dialErr)
		h.metrics.ConnectAttempts.WithLabelValues(pending.Protocol, failure.metricLabel).Inc()
		return Fail(c, failure.err)
	}

	initialDir, wdErr := client.WorkingDir()
	if wdErr != nil {
		_ = client.Close()
		return Fail(c, gftperrors.New(gftperrors.ErrConnectionFailed, "could not get working directory").WithCause(wdErr))
	}

	disableChmod := !client.SupportsChmod()

	sess.Set("client", client)
	sess.Set("initialDir", initialDir)
	sess.Set("disableChmod", disableChmod)
	// For access-log and metrics enrichment only, never the password.
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

package api

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/darthsoup/goblinftp/internal/config"

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

func pendingCookie(c echo.Context, cfg *config.Config, value string, maxAge int) *http.Cookie {
	ck := sessionCookie(c, cfg, value, maxAge) //nolint:gosec // G124: pending cookies use the same security policy as active cookies.
	ck.Name = strings.Replace(ck.Name, "_session", "_sso", 1)
	return ck
}

func (h *Handler) pendingSession(c echo.Context) (*auth.Session, bool) {
	sess, ok := lookupNamedSession(c, h.store, pendingCookie(c, h.cfg, "", 0).Name)
	if !ok {
		return nil, false
	}
	_, ok = sess.Get(ssoPendingKey)
	return sess, ok
}

func (h *Handler) SSOLogin(c echo.Context) error {
	raw := c.QueryParam("sso")
	if raw == "" {
		return c.String(http.StatusOK, h.cfg.Settings.Branding.AppName)
	}
	if !h.cfg.SSOEnabled {
		return h.ssoReject(c, "disabled", nil)
	}
	payload, err := sso.Decrypt(raw, h.cfg.SSOSecret)
	if err != nil {
		reason := "invalid"
		if errors.Is(err, sso.ErrTokenExpired) {
			reason = "expired"
		}
		return h.ssoReject(c, reason, err)
	}
	if !isAllowedType(payload.Type, h.cfg.Settings.Connection.AllowedTypes) || payload.Host == "" || len(payload.Host) > 255 || len(payload.Username) > 1024 || payload.Port < 1 || payload.Port > 65535 {
		return h.ssoReject(c, "invalid", nil)
	}
	if gerr := h.checkHostLock(payload.Host, payload.Port); gerr != nil {
		return h.ssoReject(c, "invalid", gerr)
	}
	if gerr := h.checkIPAllowlist(c); gerr != nil {
		return h.ssoReject(c, "invalid", gerr)
	}
	if !h.ssoUsed.MarkIfUnused(tokenHash(raw), time.Unix(payload.Exp, 0)) {
		return h.ssoReject(c, "used", nil)
	}
	free, allowed := h.store.Reserve(c.RealIP(), h.cfg.MaxSessions, h.cfg.MaxSessionsPerIP, h.cfg.MaxConnecting, h.cfg.MaxConnectingPerIP)
	if !allowed {
		return h.ssoReject(c, "capacity", nil)
	}
	defer free()
	settle, allowed := h.throttle.Reserve("sso:"+c.RealIP(), "ip:"+c.RealIP(), h.cfg.LoginMaxAttempts, time.Duration(h.cfg.LoginCooldownSeconds)*time.Second)
	if !allowed {
		return h.ssoReject(c, "capacity", nil)
	}
	defer settle(false)

	csrf, err := auth.GenerateCSRFToken()
	if err != nil {
		return h.ssoReject(c, "internal", err)
	}
	sess, err := h.store.New()
	if err != nil {
		return h.ssoReject(c, "internal", err)
	}
	expiry := time.Now().Add(5 * time.Minute)
	if tokenExpiry := time.Unix(payload.Exp, 0); tokenExpiry.Before(expiry) {
		expiry = tokenExpiry
	}
	h.store.SetExpiry(sess.ID, expiry)
	sess.Set("clientIP", c.RealIP())
	sess.Set(auth.CSRFSessionKey, csrf)
	sess.Set(ssoPendingKey, *payload)
	if old, ok := h.pendingSession(c); ok {
		h.store.Delete(old.ID)
	}
	writeSessionCookie(c, pendingCookie(c, h.cfg, sess.ID, 0))
	return c.Redirect(http.StatusFound, "/login?sso_pending=1")
}

func (h *Handler) SSOPending(c echo.Context) error {
	sess, ok := h.pendingSession(c)
	if !ok {
		return OK(c, map[string]bool{"pending": false})
	}
	value, _ := sess.Get(ssoPendingKey)
	payload, ok := value.(sso.Payload)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "invalid pending login"))
	}
	return OK(c, map[string]any{"pending": true, "protocol": payload.Type, "host": payload.Host, "port": payload.Port, "username": payload.Username, "language": payload.Language, "csrfToken": sess.GetString(auth.CSRFSessionKey)})
}

func (h *Handler) requirePending(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, ok := h.pendingSession(c)
		if !ok || !auth.ValidateCSRFToken(sess.GetString(auth.CSRFSessionKey), c.Request().Header.Get(auth.CSRFHeaderName)) {
			return Fail(c, gftperrors.New(gftperrors.ErrCSRFInvalid, "invalid pending login"))
		}
		c.Set("session", sess)
		return next(c)
	}
}

func (h *Handler) SSOCancel(c echo.Context) error {
	sess, _ := c.Get("session").(*auth.Session)
	if !sess.TryLockTransfer() {
		return Fail(c, gftperrors.New(gftperrors.ErrCapacity, "login is already being finalized"))
	}
	defer sess.UnlockTransfer()
	h.store.Delete(sess.ID)
	writeSessionCookie(c, pendingCookie(c, h.cfg, "", -1))
	return OK(c, nil)
}

// AuthStatus handles GET /api/auth/status. Public: it reads the session cookie
// itself. With ?ping=1 a dead connection is closed and reported as connected=false.
func (h *Handler) AuthStatus(c echo.Context) error {
	type statusData struct {
		ConnectionID     string        `json:"connectionId,omitempty"`
		Language         string        `json:"language,omitempty"`
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
		result.SSOAutoConnect = false
		result.CSRFToken = sess.GetString(auth.CSRFSessionKey)

		// Connection context for SPA state restoration after a reload.
		if result.Connected {
			result.ConnectionID = sess.ConnectionID
			result.Language = sess.GetString("language")
			result.Host = sess.GetString("host")
			result.InitialDirectory = sess.GetString("initialDir")
			disableChmodVal, _ := sess.Get("disableChmod")
			disableChmod, _ := disableChmodVal.(bool)
			result.Capabilities = &Capabilities{DisableChmod: disableChmod}
		}
	}

	return OK(c, result)
}

func (h *Handler) SSOConnect(c echo.Context) error {
	pendingSession, _ := c.Get("session").(*auth.Session)
	if !pendingSession.TryLockTransfer() {
		return Fail(c, gftperrors.New(gftperrors.ErrCapacity, "login is already being finalized"))
	}
	defer pendingSession.UnlockTransfer()
	value, ok := pendingSession.Get(ssoPendingKey)
	payload, valid := value.(sso.Payload)
	if !ok || !valid || time.Now().Unix() >= payload.Exp {
		return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "pending login expired"))
	}
	var body struct {
		AcceptHostKey string `json:"acceptHostKey"`
	}
	if gerr := bindJSON(c, &body); gerr != nil {
		return Fail(c, gerr)
	}
	if gerr := h.checkIPAllowlist(c); gerr != nil {
		return Fail(c, gerr)
	}
	settle, allowed := h.throttle.Reserve(accountThrottleKey(payload.Host, payload.Username), "ip:"+c.RealIP(), h.cfg.LoginMaxAttempts, time.Duration(h.cfg.LoginCooldownSeconds)*time.Second)
	if !allowed {
		return Fail(c, gftperrors.New(gftperrors.ErrLoginThrottled, "too many connection attempts"))
	}
	defer settle(false)
	free, allowed := h.store.Reserve(c.RealIP(), h.cfg.MaxSessions, h.cfg.MaxSessionsPerIP, h.cfg.MaxConnecting, h.cfg.MaxConnectingPerIP, pendingSession.ID)
	if !allowed {
		return Fail(c, gftperrors.New(gftperrors.ErrCapacity, "connection capacity reached"))
	}
	defer free()
	addr := net.JoinHostPort(payload.Host, strconv.Itoa(payload.Port))
	client, hostKey, err := h.dial(DialRequest{Context: c.Request().Context(), Protocol: payload.Type, Addr: addr, Host: payload.Host, User: payload.Username, Pass: payload.Password, Passive: h.cfg.Settings.Connection.PassiveMode, AcceptHostKey: body.AcceptHostKey})
	if hostKey != nil {
		noteHostKeyChange(c, h.logger, payload.Host, hostKey)
		return OK(c, ConnectData{HostKeyPrompt: hostKey})
	}
	if err != nil {
		failure := classifyDial(err)
		settle(failure.credentialAttempt)
		return Fail(c, failure.err)
	}
	success := false
	defer func() {
		if !success {
			_ = client.Close()
		}
	}()
	initialDir, err := client.WorkingDir()
	if err != nil {
		return failClient(c, gftperrors.ErrConnectionFailed, err)
	}
	if payload.InitialDirectory != "" {
		if err := transfer.ValidateRemotePath(payload.InitialDirectory, payload.Type != "sftp", true); err != nil {
			return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "invalid initial directory"))
		}
		info, err := client.Stat(payload.InitialDirectory)
		if err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		if !info.IsDir {
			return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "initial directory is not a directory"))
		}
		initialDir = payload.InitialDirectory
	}
	csrf, err := auth.GenerateCSRFToken()
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "could not generate CSRF token"))
	}
	sess, err := h.store.New()
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "could not create session"))
	}
	sess.Set("clientIP", c.RealIP())
	sess.Set("client", client)
	sess.Set(auth.CSRFSessionKey, csrf)
	sess.Set("initialDir", initialDir)
	sess.Set("disableChmod", !client.SupportsChmod())
	sess.Set("username", payload.Username)
	sess.Set("host", addr)
	sess.Set("protocol", payload.Type)
	sess.Set("language", payload.Language)
	if tenant := sanitizeTenant(payload.Tenant); tenant != "" {
		sess.Set(tenantSessionKey, tenant)
	}
	if old, ok := lookupSession(c, h.store); ok {
		old.LockTransfer()
		h.evictSession(old)
		old.UnlockTransfer()
		h.store.Delete(old.ID)
	}
	pendingSession.Delete(ssoPendingKey)
	h.store.Delete(pendingSession.ID)
	writeSessionCookie(c, pendingCookie(c, h.cfg, "", -1))
	setSessionCookie(c, h.cfg, sess.ID, 0)
	success = true
	h.metrics.ConnectAttempts.WithLabelValues(payload.Type, "success").Inc()
	return OK(c, ConnectData{ConnectionID: sess.ConnectionID, Host: addr, Language: payload.Language, InitialDirectory: initialDir, CSRFToken: csrf, Capabilities: Capabilities{DisableChmod: !client.SupportsChmod()}})
}

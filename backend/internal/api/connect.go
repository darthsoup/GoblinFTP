package api

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
)

// ConnectRequest is the JSON body for POST /api/auth/connect.
type ConnectRequest struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Passive  bool   `json:"passive"`
	// AcceptHostKey is the SHA256 fingerprint the user agreed to trust for an
	// unknown SFTP host (trust-on-first-use, step 2). Empty on the first attempt.
	AcceptHostKey string `json:"acceptHostKey"`
}

// ConnectData is the successful response payload for POST /api/auth/connect.
type ConnectData struct {
	ConnectionID     string       `json:"connectionId,omitempty"`
	Host             string       `json:"host,omitempty"`
	Language         string       `json:"language,omitempty"`
	Capabilities     Capabilities `json:"capabilities"`
	InitialDirectory string       `json:"initialDirectory"`
	CSRFToken        string       `json:"csrfToken"`
	// HostKeyPrompt is set (with the other fields empty and no session created)
	// when an SFTP host key must be confirmed before connecting.
	HostKeyPrompt *HostKeyPrompt `json:"hostKeyPrompt,omitempty"`
}

// Capabilities describes what the connected server supports.
type Capabilities struct {
	DisableChmod bool `json:"disableChmod"`
}

// Connect handles POST /api/auth/connect.
func (h *Handler) Connect(c echo.Context) error {
	var req ConnectRequest
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}

	// Enforced server-side, not only hidden in the SPA: without this an SSO-only
	// instance is still an unauthenticated dialer for arbitrary hosts. Placed
	// before the throttle so a rejected request consumes no login attempts, and
	// deliberately not on the SSOConnect path.
	if h.cfg.LoginFormDisabled {
		return Fail(c, gftperrors.New(gftperrors.ErrLoginDisabled,
			"manual login is disabled on this server"))
	}

	if !isAllowedType(req.Protocol, h.cfg.Settings.Connection.AllowedTypes) {
		return Fail(c, gftperrors.New(gftperrors.ErrInvalidType,
			fmt.Sprintf("connection type %q is not allowed", req.Protocol)))
	}

	conn := h.cfg.Settings.Connection
	if conn.LockHost && conn.PresetHost != nil {
		if req.Host == "" {
			req.Host = *conn.PresetHost
		}
		if req.Port == 0 && conn.PresetPort != nil {
			req.Port = *conn.PresetPort
		}
		if req.Port == 0 {
			req.Port = 21
			if req.Protocol == "sftp" {
				req.Port = 22
			}
		}
	}
	if req.Host == "" || req.Username == "" || len(req.Host) > 255 || len(req.Username) > 1024 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "host and username are required"))
	}
	if req.Port <= 0 || req.Port > 65535 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "port must be between 1 and 65535"))
	}

	// Enforce the host lock server-side: LockHost only disabled the SPA's host
	// field, so a direct POST could dial anywhere and probe the internal network.
	if gftperr := h.checkHostLock(req.Host, req.Port); gftperr != nil {
		return Fail(c, gftperr)
	}

	if gftperr := h.checkIPAllowlist(c); gftperr != nil {
		return Fail(c, gftperr)
	}

	// Two throttle dimensions: host+username alone is evaded by varying the
	// username (a password spray), so the client IP is counted too, on a wider budget.
	throttleKey := accountThrottleKey(req.Host, req.Username)
	ipKey := "ip:" + c.RealIP()
	free, allowed := h.store.Reserve(c.RealIP(), h.cfg.MaxSessions, h.cfg.MaxSessionsPerIP, h.cfg.MaxConnecting, h.cfg.MaxConnectingPerIP)
	if !allowed {
		return Fail(c, gftperrors.New(gftperrors.ErrCapacity, "connection capacity reached"))
	}
	defer free()
	settle, allowed := h.throttle.Reserve(throttleKey, ipKey, h.cfg.LoginMaxAttempts, time.Duration(h.cfg.LoginCooldownSeconds)*time.Second)
	if !allowed {
		h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "throttled").Inc()
		return Fail(c, gftperrors.New(gftperrors.ErrLoginThrottled, "too many connection attempts, please try again later"))
	}
	defer settle(false)

	addr := net.JoinHostPort(req.Host, strconv.Itoa(req.Port))
	client, hostKey, dialErr := h.dial(DialRequest{
		Context:       c.Request().Context(),
		Protocol:      req.Protocol,
		Addr:          addr,
		Host:          req.Host,
		User:          req.Username,
		Pass:          req.Password,
		Passive:       req.Passive,
		AcceptHostKey: req.AcceptHostKey,
	})
	if hostKey != nil {
		// Host-key prompts consume IP admission without recording a credential failure.
		h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, hostKeyLabel(hostKey)).Inc()
		noteHostKeyChange(c, h.logger, req.Host, hostKey)
		return OK(c, ConnectData{HostKeyPrompt: hostKey})
	}
	if dialErr != nil {
		failure := classifyDial(dialErr)
		settle(failure.credentialAttempt)
		h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, failure.metricLabel).Inc()
		return Fail(c, failure.err)
	}

	initialDir, wdErr := client.WorkingDir()
	if wdErr != nil {
		_ = client.Close()
		return Fail(c, gftperrors.New(gftperrors.ErrConnectionFailed, "could not get working directory").WithCause(wdErr))
	}

	if gerr := validateServerDirectory(initialDir, req.Protocol); gerr != nil {
		_ = client.Close()
		return Fail(c, gerr)
	}

	disableChmod := !client.SupportsChmod()

	csrfToken, csrfErr := auth.GenerateCSRFToken()
	if csrfErr != nil {
		_ = client.Close()
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "could not generate CSRF token").WithCause(csrfErr))
	}

	sess, sessErr := h.store.New()
	if sessErr != nil {
		_ = client.Close()
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "could not create session").WithCause(sessErr))
	}
	sess.Set("clientIP", c.RealIP())
	sess.Set("client", client)
	sess.Set(auth.CSRFSessionKey, csrfToken)
	sess.Set("initialDir", initialDir)
	sess.Set("disableChmod", disableChmod)
	// For access-log and metrics enrichment only, never the password.
	sess.Set("username", req.Username)
	sess.Set("host", addr)
	sess.Set("protocol", req.Protocol)

	h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "success").Inc()

	if old, ok := lookupSession(c, h.store); ok && old.ID != sess.ID {
		old.LockTransfer()
		h.evictSession(old)
		old.UnlockTransfer()
		h.store.Delete(old.ID)
	}
	setSessionCookie(c, h.cfg, sess.ID, 0)

	return OK(c, ConnectData{
		ConnectionID: sess.ConnectionID, Host: addr,
		Capabilities:     Capabilities{DisableChmod: disableChmod},
		InitialDirectory: initialDir,
		CSRFToken:        csrfToken,
	})
}

// Disconnect handles POST /api/auth/disconnect.
// Requires a valid session (enforced by requireSession middleware in router.go).
func (h *Handler) Disconnect(c echo.Context) error {
	sess, ok := c.Get("session").(*auth.Session)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active session"))
	}
	// Hold the transfer lock so a disconnect can't close the connection out
	// from under an in-flight transfer mid-data-stream. evictSession, not a bare
	// close: an explicit disconnect must release staged chunks as well, or they
	// sit on the volume until the 24h sweeper reclaims them.
	sess.LockTransfer()
	h.evictSession(sess)
	sess.UnlockTransfer()
	h.store.Delete(sess.ID)
	setSessionCookie(c, h.cfg, "", -1)
	return OK(c, nil)
}

func isAllowedType(t string, allowed []string) bool {
	for _, a := range allowed {
		if strings.EqualFold(a, t) {
			return true
		}
	}
	return false
}

// checkHostLock rejects a target differing from the GFTP_CONNECTION_LOCK_HOST
// preset. An unset PresetHost means the lock is off; PresetPort binds only if set.
func (h *Handler) checkHostLock(host string, port int) *gftperrors.GFTPError {
	conn := h.cfg.Settings.Connection
	if !conn.LockHost || conn.PresetHost == nil || *conn.PresetHost == "" {
		return nil
	}
	if !strings.EqualFold(strings.TrimSuffix(host, "."), strings.TrimSuffix(*conn.PresetHost, ".")) {
		return gftperrors.New(gftperrors.ErrForbidden, "this instance is locked to a fixed host")
	}
	if conn.PresetPort != nil && port != *conn.PresetPort {
		return gftperrors.New(gftperrors.ErrForbidden, "this instance is locked to a fixed port")
	}
	return nil
}

func (h *Handler) checkIPAllowlist(c echo.Context) *gftperrors.GFTPError {
	allowed := h.cfg.Settings.Access.AllowedClientAddresses
	if len(allowed) == 0 {
		return nil
	}
	// c.RealIP() follows the IPExtractor from newApp: the direct peer unless
	// GFTP_ACCESS_TRUSTED_PROXIES names the proxy, else every client is the proxy.
	clientIP := net.ParseIP(c.RealIP())
	if clientIP == nil {
		return gftperrors.New(gftperrors.ErrForbidden, "client IP address is not in the allowlist")
	}
	for _, addr := range allowed {
		if strings.Contains(addr, "/") {
			if _, ipnet, err := net.ParseCIDR(addr); err == nil && ipnet.Contains(clientIP) {
				return nil
			}
			continue
		}
		if allowedIP := net.ParseIP(addr); allowedIP != nil && allowedIP.Equal(clientIP) {
			return nil
		}
	}
	return gftperrors.New(gftperrors.ErrForbidden, "client IP address is not in the allowlist")
}

func accountThrottleKey(host, user string) string {
	return "account:" + strconv.Quote(strings.ToLower(strings.TrimSuffix(host, "."))) + ":" + strconv.Quote(user)
}

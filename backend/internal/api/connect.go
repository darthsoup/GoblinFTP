// backend/internal/api/connect.go
package api

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
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
// Populated in Phase 3 once the FTP/SFTP connection is established.
type ConnectData struct {
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

// loginIPAttemptMultiplier widens the per-IP budget relative to the
// per-host+username one: many users can legitimately share an egress address.
const loginIPAttemptMultiplier = 4

// Connect handles POST /api/auth/connect.
// Phase 2: validates input, checks IP allowlist, checks throttle. Returns 501.
// Phase 3: adds actual FTP/SFTP connection and session creation.
func (h *Handler) Connect(c echo.Context) error {
	var req ConnectRequest
	if err := c.Bind(&req); err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "invalid request body"))
	}

	// Validate connection type
	if !isAllowedType(req.Protocol, h.cfg.Settings.Connection.AllowedTypes) {
		return Fail(c, gftperrors.New(gftperrors.ErrInvalidType,
			fmt.Sprintf("connection type %q is not allowed", req.Protocol)))
	}

	// Validate required fields
	if req.Host == "" || req.Username == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "host and username are required"))
	}
	if req.Port <= 0 || req.Port > 65535 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "port must be between 1 and 65535"))
	}

	// Enforce the host lock server-side. LockHost only ever disabled the SPA's
	// host field, so a direct POST could still dial anywhere - both a bypass of
	// the operator's policy and an unauthenticated way to probe the internal
	// network, since the error codes distinguish refused from auth-failed.
	if gftperr := h.checkHostLock(req.Host, req.Port); gftperr != nil {
		return Fail(c, gftperr)
	}

	// Check IP allowlist
	if gftperr := h.checkIPAllowlist(c); gftperr != nil {
		return Fail(c, gftperr)
	}

	// Two throttle dimensions. host+username alone is trivially evaded by
	// varying the username, which is exactly the shape of a password spray, so
	// the client IP is counted too and gets a wider budget (a shared NAT egress
	// is one address for many legitimate users).
	throttleKey := req.Host + ":" + req.Username
	ipKey := "ip:" + c.RealIP()
	ipMaxAttempts := h.cfg.LoginMaxAttempts * loginIPAttemptMultiplier
	if h.throttle.IsThrottled(throttleKey, h.cfg.LoginMaxAttempts) ||
		h.throttle.IsThrottled(ipKey, ipMaxAttempts) {
		h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "throttled").Inc()
		return Fail(c, gftperrors.New(gftperrors.ErrLoginThrottled,
			"too many failed login attempts, please try again later"))
	}

	addr := fmt.Sprintf("%s:%d", req.Host, req.Port)
	client, hostKey, dialErr := h.dial(DialRequest{
		Protocol:      req.Protocol,
		Addr:          addr,
		Host:          req.Host,
		User:          req.Username,
		Pass:          req.Password,
		Passive:       req.Passive,
		AcceptHostKey: req.AcceptHostKey,
	})
	if hostKey != nil {
		// Unknown SFTP host key - ask the user to confirm before we connect or
		// send credentials. Not a failed attempt, so the throttle is untouched.
		h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "host_key_prompt").Inc()
		return OK(c, ConnectData{HostKeyPrompt: hostKey})
	}
	if dialErr != nil {
		cooldown := time.Duration(h.cfg.LoginCooldownSeconds) * time.Second
		h.throttle.Record(throttleKey, cooldown)
		h.throttle.Record(ipKey, cooldown)
		switch {
		case errors.Is(dialErr, transfer.ErrAuthFailed):
			h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "auth_failed").Inc()
			return Fail(c, gftperrors.New(gftperrors.ErrAuthFailed, "authentication failed").WithCause(dialErr))
		case errors.Is(dialErr, transfer.ErrHostKeyMismatch):
			h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "host_key_mismatch").Inc()
			return Fail(c, gftperrors.New(gftperrors.ErrHostKeyMismatch,
				"the server's host key changed since it was trusted - possible man-in-the-middle, connection refused").WithCause(dialErr))
		case errors.Is(dialErr, transfer.ErrTLSFailed):
			h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "tls_failed").Inc()
			return Fail(c, gftperrors.New(gftperrors.ErrTLSFailed,
				"the server's TLS certificate could not be verified").WithCause(dialErr))
		default:
			h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "failed").Inc()
			return Fail(c, gftperrors.New(gftperrors.ErrConnectionFailed, "could not connect to server").WithCause(dialErr))
		}
	}
	h.throttle.Reset(throttleKey)
	h.throttle.Reset(ipKey)

	initialDir, wdErr := client.WorkingDir()
	if wdErr != nil {
		_ = client.Close()
		return Fail(c, gftperrors.New(gftperrors.ErrConnectionFailed, "could not get working directory").WithCause(wdErr))
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
	sess.Set("client", client)
	sess.Set(auth.CSRFSessionKey, csrfToken)
	sess.Set("initialDir", initialDir)
	sess.Set("disableChmod", disableChmod)
	// For access-log and metrics enrichment only - never the password.
	sess.Set("username", req.Username)
	sess.Set("host", addr)
	sess.Set("protocol", req.Protocol)

	h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "success").Inc()

	c.SetCookie(sessionCookie(c, h.cfg, sess.ID, 0))

	return OK(c, ConnectData{
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
	// from under an in-flight transfer mid-data-stream.
	sess.LockTransfer()
	closeSessionClient(sess)
	sess.UnlockTransfer()
	h.store.Delete(sess.ID)
	c.SetCookie(sessionCookie(c, h.cfg, "", -1))
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

// checkHostLock rejects a target that differs from the configured preset when
// GFTP_CONNECTION_LOCK_HOST is set. Config guarantees PresetHost is non-empty
// whenever LockHost is true, so an unset preset here means the lock is off.
// PresetPort is only enforced when it is also configured: locking the host
// without pinning a port is a valid setup.
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
	// c.RealIP() is governed by the IPExtractor installed in newApp: the direct
	// peer unless GFTP_ACCESS_TRUSTED_PROXIES names the proxy in front. Behind
	// an untrusted-but-present proxy every client looks like the proxy, which
	// is why that key exists.
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

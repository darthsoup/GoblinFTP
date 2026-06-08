// backend/internal/api/connect.go
package api

import (
	"errors"
	"fmt"
	"net/http"
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
}

// ConnectData is the successful response payload for POST /api/auth/connect.
// Populated in Phase 3 once the FTP/SFTP connection is established.
type ConnectData struct {
	Capabilities     Capabilities `json:"capabilities"`
	InitialDirectory string       `json:"initialDirectory"`
	CSRFToken        string       `json:"csrfToken"`
}

// Capabilities describes what the connected server supports.
type Capabilities struct {
	DisableChmod bool `json:"disableChmod"`
}

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

	// Check IP allowlist
	if gftperr := h.checkIPAllowlist(c); gftperr != nil {
		return Fail(c, gftperr)
	}

	// Check login throttle
	throttleKey := req.Host + ":" + req.Username
	if h.throttle.IsThrottled(throttleKey, h.cfg.LoginMaxAttempts) {
		h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "throttled").Inc()
		return Fail(c, gftperrors.New(gftperrors.ErrLoginThrottled,
			"too many failed login attempts, please try again later"))
	}

	addr := fmt.Sprintf("%s:%d", req.Host, req.Port)
	client, dialErr := h.dial(req.Protocol, addr, req.Username, req.Password, req.Passive)
	if dialErr != nil {
		h.throttle.Record(throttleKey, time.Duration(h.cfg.LoginCooldownSeconds)*time.Second)
		if errors.Is(dialErr, transfer.ErrAuthFailed) {
			h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "auth_failed").Inc()
			return Fail(c, gftperrors.New(gftperrors.ErrAuthFailed, "authentication failed").WithCause(dialErr))
		}
		h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "failed").Inc()
		return Fail(c, gftperrors.New(gftperrors.ErrConnectionFailed, "could not connect to server").WithCause(dialErr))
	}
	h.throttle.Reset(throttleKey)

	initialDir, wdErr := client.WorkingDir()
	if wdErr != nil {
		_ = client.Close()
		return Fail(c, gftperrors.New(gftperrors.ErrConnectionFailed, "could not get working directory").WithCause(wdErr))
	}

	disableChmod := detectChmod(client, req.Protocol, initialDir)

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
	// For access-log and metrics enrichment only — never the password.
	sess.Set("username", req.Username)
	sess.Set("host", addr)
	sess.Set("protocol", req.Protocol)

	h.metrics.ConnectAttempts.WithLabelValues(req.Protocol, "success").Inc()

	c.SetCookie(&http.Cookie{ //nolint:gosec // G124: Secure is set conditionally below — literal true would break plain-HTTP deployments
		Name:     SessionCookieName,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		// Secure when served over TLS (directly or behind a proxy setting
		// X-Forwarded-Proto); plain-HTTP LAN deployments keep working.
		Secure:   c.Scheme() == "https",
		SameSite: http.SameSiteLaxMode,
	})

	return OK(c, ConnectData{
		Capabilities:     Capabilities{DisableChmod: disableChmod},
		InitialDirectory: initialDir,
		CSRFToken:        csrfToken,
	})
}

// Disconnect handles POST /api/auth/disconnect.
// Requires a valid session (enforced by requireSession middleware in router.go).
func (h *Handler) Disconnect(c echo.Context) error {
	sess := c.Get("session").(*auth.Session)
	if client, ok := sess.Get("client"); ok {
		if c, ok := client.(transfer.Client); ok {
			// Hold the transfer lock so a disconnect can't close the connection
			// out from under an in-flight transfer mid-data-stream.
			sess.LockTransfer()
			_ = c.Close()
			sess.UnlockTransfer()
		}
	}
	h.store.Delete(sess.ID)
	c.SetCookie(&http.Cookie{ //nolint:gosec // G124: Secure is set conditionally below — literal true would break plain-HTTP deployments
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.Scheme() == "https",
		SameSite: http.SameSiteLaxMode,
	})
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

func (h *Handler) checkIPAllowlist(c echo.Context) *gftperrors.GFTPError {
	allowed := h.cfg.Settings.Access.AllowedClientAddresses
	if len(allowed) == 0 {
		return nil
	}
	// c.RealIP() reads X-Forwarded-For first (set by Caddy in production).
	// This is safe because the Go binary only listens on localhost inside the container,
	// so only Caddy can send requests and control the XFF header.
	clientIP := c.RealIP()
	for _, addr := range allowed {
		if addr == clientIP {
			return nil
		}
	}
	return gftperrors.New(gftperrors.ErrForbidden, "client IP address is not in the allowlist")
}

// detectChmod probes whether the server supports chmod operations.
func detectChmod(client transfer.Client, protocol, dir string) bool {
	if protocol == "ftp" {
		return false // assume FTP servers support SITE CHMOD
	}
	err := client.Chmod(dir, 0o755)
	return errors.Is(err, transfer.ErrPermissionsNotSupported)
}

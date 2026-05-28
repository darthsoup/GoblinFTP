// backend/internal/api/connect.go
package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
)

// ConnectRequest is the JSON body for POST /api/auth/connect.
type ConnectRequest struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// ConnectData is the successful response payload for POST /api/auth/connect.
// Populated in Phase 3 once the FTP/SFTP connection is established.
type ConnectData struct {
	Capabilities     ConnectCapabilities `json:"capabilities"`
	InitialDirectory string              `json:"initialDirectory"`
	CSRFToken        string              `json:"csrfToken"`
}

// ConnectCapabilities describes what the connected server supports.
type ConnectCapabilities struct {
	Chmod bool `json:"chmod"`
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
	if !isAllowedType(req.Type, h.cfg.Settings.Connection.AllowedTypes) {
		return Fail(c, gftperrors.New(gftperrors.ErrInvalidType,
			fmt.Sprintf("connection type %q is not allowed", req.Type)))
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
		return Fail(c, gftperrors.New(gftperrors.ErrLoginThrottled,
			"too many failed login attempts, please try again later"))
	}

	// Phase 3: FTP/SFTP connection goes here.
	return Fail(c, gftperrors.New(gftperrors.ErrNotImplemented,
		"FTP/SFTP connection not implemented in this phase"))
}

// Disconnect handles POST /api/auth/disconnect.
// Requires a valid session (enforced by requireSession middleware in router.go).
func (h *Handler) Disconnect(c echo.Context) error {
	sess := c.Get("session").(*auth.Session)
	h.store.Delete(sess.ID)
	c.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
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

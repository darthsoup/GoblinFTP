// backend/internal/api/router_test.go
package api_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
)

func newTestApp(t *testing.T, cfg *config.Config, opts ...api.HandlerOption) (*echo.Echo, *auth.Store, *auth.Throttle) {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	store := auth.NewStore(time.Duration(cfg.SessionTTLSeconds) * time.Second)
	thr := auth.NewThrottle()
	api.Register(e, cfg, store, thr, opts...)
	return e, store, thr
}

// newTestAppWithLog is newTestApp with a debug-level JSON logger writing into
// buf, for tests asserting on access-log lines (decode buf line by line).
func newTestAppWithLog(t *testing.T, cfg *config.Config, buf *bytes.Buffer, opts ...api.HandlerOption) (*echo.Echo, *auth.Store, *auth.Throttle) {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return newTestApp(t, cfg, append(opts, api.WithLogger(logger))...)
}

// logLines decodes every JSON log line in buf.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("invalid log line %q: %v", raw, err)
		}
		lines = append(lines, m)
	}
	return lines
}

func defaultTestConfig() *config.Config {
	return &config.Config{
		Port:                 "8080",
		LogLevel:             "info",
		FrontendLogEnabled:   true,
		SessionSecret:        []byte("test-session-secret"),
		DownloadTokenSecret:  []byte("test-download-secret"),
		LoginMaxAttempts:     5,
		LoginCooldownSeconds: 300,
		SessionTTLSeconds:    7200,
		ChunkSize:            5 * 1024 * 1024,
		DataDir:              os.TempDir(),
		Settings: config.Settings{
			Connection: config.ConnectionSettings{
				AllowedTypes:          []string{"ftp", "sftp"},
				DisableChmod:          false,
				RequestTimeoutSeconds: 30,
			},
			Access: config.AccessSettings{
				AllowedClientAddresses: []string{},
			},
		},
	}
}

func TestRequireSessionMiddlewareRejectsUnauthenticated(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireSessionMiddlewareAllowsValidSession(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	sess, err := store.New()
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Session exists, but no client -> handler returns 401 (no active connection)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestCSRFMiddlewareBlocksMutatingRequestsWithoutToken(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	sess, err := store.New()
	assert.NoError(t, err)

	csrfToken, err := auth.GenerateCSRFToken()
	assert.NoError(t, err)
	sess.Data[auth.CSRFSessionKey] = csrfToken

	// POST without X-CSRF-Token header
	req := httptest.NewRequest(http.MethodPost, "/api/auth/disconnect", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestCSRFMiddlewareAllowsMutatingRequestsWithValidToken(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	sess, err := store.New()
	assert.NoError(t, err)

	csrfToken, err := auth.GenerateCSRFToken()
	assert.NoError(t, err)
	sess.Data[auth.CSRFSessionKey] = csrfToken

	req := httptest.NewRequest(http.MethodPost, "/api/auth/disconnect", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.CSRFHeaderName, csrfToken)
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddlewareSkipsGETRequests(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	sess, err := store.New()
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// GET doesn't need CSRF token. Session exists but no client -> 401
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHealthzNotAffectedByAPIMiddleware(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

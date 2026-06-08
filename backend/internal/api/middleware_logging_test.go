// backend/internal/api/middleware_logging_test.go
package api_test

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

// requestLines filters decoded log lines down to access-log entries.
func requestLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range logLines(t, buf) {
		if line["msg"] == "request" {
			out = append(out, line)
		}
	}
	return out
}

func TestAccessLogUnauthenticatedRequest(t *testing.T) {
	var buf bytes.Buffer
	e, store, _ := newTestAppWithLog(t, defaultTestConfig(), &buf)
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	lines := requestLines(t, &buf)
	require.Len(t, lines, 1, "exactly one access line per request")
	line := lines[0]

	assert.Equal(t, "WARN", line["level"])
	assert.Equal(t, "GET", line["method"])
	assert.Equal(t, "/api/files", line["path"])
	assert.Equal(t, float64(http.StatusUnauthorized), line["status"])
	assert.Equal(t, "ERR_UNAUTHORIZED", line["error_code"])
	assert.Equal(t, "not authenticated", line["error"])
	assert.NotEmpty(t, line["request_id"])
	assert.NotEmpty(t, line["remote_ip"])
	assert.Contains(t, line, "duration_ms")
	assert.Greater(t, line["bytes_out"], float64(0))
}

func TestAccessLogSessionEnrichment(t *testing.T) {
	var buf bytes.Buffer
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
		ListFn:       func(path string) ([]transfer.FileInfo, error) { return nil, nil },
	}
	e, store, _ := newTestAppWithLog(t, defaultTestConfig(), &buf, api.WithDial(staticDial(mock)))
	defer store.Close()

	sess := connectAndGetSession(t, e)
	buf.Reset() // discard the connect line; assert on the authed request only

	req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	lines := requestLines(t, &buf)
	require.Len(t, lines, 1)
	line := lines[0]

	assert.Equal(t, "INFO", line["level"])
	assert.Equal(t, "u", line["user"])
	assert.Equal(t, "h:21", line["host"])

	// Only an 8-char prefix of the session ID may appear — the full ID is the
	// bearer cookie value.
	sessionField, ok := line["session"].(string)
	require.True(t, ok, "session field missing")
	assert.Len(t, sessionField, 8)
	for _, ck := range sess.cookies {
		if ck.Name == api.SessionCookieName {
			assert.Equal(t, ck.Value[:8], sessionField)
			assert.NotEqual(t, ck.Value, sessionField, "full session ID must never be logged")
		}
	}
	assert.NotContains(t, line, "password")
}

func TestAccessLogHealthzAtDebug(t *testing.T) {
	var buf bytes.Buffer
	e, store, _ := newTestAppWithLog(t, defaultTestConfig(), &buf)
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	lines := requestLines(t, &buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "DEBUG", lines[0]["level"])
}

func TestAccessLogConnLostCarriesCause(t *testing.T) {
	var buf bytes.Buffer
	brokenPipe := &net.OpError{Op: "write", Net: "tcp", Err: syscall.EPIPE}
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
		ListFn:       func(path string) ([]transfer.FileInfo, error) { return nil, brokenPipe },
	}
	e, store, _ := newTestAppWithLog(t, defaultTestConfig(), &buf, api.WithDial(staticDial(mock)))
	defer store.Close()

	sess := connectAndGetSession(t, e)
	buf.Reset()

	req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadGateway, rec.Code)

	lines := requestLines(t, &buf)
	require.Len(t, lines, 1)
	line := lines[0]

	assert.Equal(t, "ERROR", line["level"])
	assert.Equal(t, "ERR_CONNECTION_LOST", line["error_code"])
	// The client-facing message stays clean; the raw cause is log-only.
	assert.NotContains(t, line["error"], "broken pipe")
	assert.Contains(t, line["cause"], "broken pipe")
}

func TestAccessLogPanicLogsAs500(t *testing.T) {
	var buf bytes.Buffer
	e, store, _ := newTestAppWithLog(t, defaultTestConfig(), &buf)
	defer store.Close()
	e.GET("/panic-test", func(echo.Context) error { panic("kaboom") })

	req := httptest.NewRequest(http.MethodGet, "/panic-test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	lines := requestLines(t, &buf)
	require.Len(t, lines, 1, "a recovered panic must still produce one access line")
	assert.Equal(t, "ERROR", lines[0]["level"])
	assert.Equal(t, float64(http.StatusInternalServerError), lines[0]["status"])

	// The panic detail goes to the structured logger, not echo's plain print.
	var panicLine map[string]any
	for _, line := range logLines(t, &buf) {
		if line["msg"] == "panic recovered" {
			panicLine = line
		}
	}
	require.NotNil(t, panicLine)
	assert.Contains(t, panicLine["error"], "kaboom")
	assert.NotEmpty(t, panicLine["stack"])
}

// backend/internal/api/frontendlog_test.go
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
)

func postFrontendLog(t *testing.T, e http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/log/frontend", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-agent/1.0")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// frontendErrorLines filters decoded log lines down to forwarded browser errors.
func frontendErrorLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range logLines(t, buf) {
		if line["msg"] == "frontend error" {
			out = append(out, line)
		}
	}
	return out
}

// TestFrontendLogHappyPath: a valid report needs no session and no CSRF token
// and lands as one WARN line with all fields.
func TestFrontendLogHappyPath(t *testing.T) {
	var buf bytes.Buffer
	e, store, _ := newTestAppWithLog(t, defaultTestConfig(), &buf)
	defer store.Close()

	rec := postFrontendLog(t, e, `{"kind":"error","message":"boom","stack":"at main.js:1","source":"main.js:1:1","route":"/files"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)

	lines := frontendErrorLines(t, &buf)
	require.Len(t, lines, 1)
	line := lines[0]
	assert.Equal(t, "WARN", line["level"])
	assert.Equal(t, "error", line["kind"])
	assert.Equal(t, "boom", line["message"])
	assert.Equal(t, "at main.js:1", line["stack"])
	assert.Equal(t, "main.js:1:1", line["source"])
	assert.Equal(t, "/files", line["route"])
	assert.NotEmpty(t, line["remote_ip"])
	assert.Equal(t, "test-agent/1.0", line["user_agent"])
}

func TestFrontendLogTruncatesLongFields(t *testing.T) {
	var buf bytes.Buffer
	e, store, _ := newTestAppWithLog(t, defaultTestConfig(), &buf)
	defer store.Close()

	// Long enough to exceed both truncation limits, but under the 16K body limit.
	long := strings.Repeat("x", 6_000)
	payload, err := json.Marshal(map[string]string{"kind": "rejection", "message": long, "stack": long})
	require.NoError(t, err)

	rec := postFrontendLog(t, e, string(payload))
	require.Equal(t, http.StatusOK, rec.Code)

	lines := frontendErrorLines(t, &buf)
	require.Len(t, lines, 1)
	assert.Len(t, lines[0]["message"], 500)
	assert.Len(t, lines[0]["stack"], 4000)
}

func TestFrontendLogRejectsUnknownKind(t *testing.T) {
	var buf bytes.Buffer
	e, store, _ := newTestAppWithLog(t, defaultTestConfig(), &buf)
	defer store.Close()

	for _, body := range []string{
		`{"kind":"sneaky","message":"boom"}`,
		`{"message":"no kind"}`,
		`not json at all`,
	} {
		rec := postFrontendLog(t, e, body)
		assert.Equal(t, http.StatusOK, rec.Code, "invalid reports are dropped silently: %s", body)
	}
	assert.Empty(t, frontendErrorLines(t, &buf))
}

// TestFrontendLogRateLimit: only 60 reports per IP per minute are accepted;
// the rest still answer 200 but produce no log line.
func TestFrontendLogRateLimit(t *testing.T) {
	var buf bytes.Buffer
	e, store, _ := newTestAppWithLog(t, defaultTestConfig(), &buf)
	defer store.Close()

	for range 61 {
		rec := postFrontendLog(t, e, `{"kind":"error","message":"spam"}`)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Len(t, frontendErrorLines(t, &buf), 60)
}

func TestFrontendLogDisabled(t *testing.T) {
	var buf bytes.Buffer
	cfg := defaultTestConfig()
	cfg.FrontendLogEnabled = false
	e, store, _ := newTestAppWithLog(t, cfg, &buf)
	defer store.Close()

	rec := postFrontendLog(t, e, `{"kind":"error","message":"boom"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, frontendErrorLines(t, &buf))
}

// TestFrontendLogOversizeBody: the 16K body limit answers 413 before the
// handler runs (echo error shape, not the envelope — acceptable for a
// defensive limit) and nothing is logged as a frontend error.
func TestFrontendLogOversizeBody(t *testing.T) {
	var buf bytes.Buffer
	e, store, _ := newTestAppWithLog(t, defaultTestConfig(), &buf)
	defer store.Close()

	payload, err := json.Marshal(map[string]string{"kind": "error", "message": strings.Repeat("x", 20_000)})
	require.NoError(t, err)

	rec := postFrontendLog(t, e, string(payload))
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Empty(t, frontendErrorLines(t, &buf))

	// The access line still records the 413.
	lines := requestLines(t, &buf)
	require.Len(t, lines, 1)
	assert.Equal(t, float64(http.StatusRequestEntityTooLarge), lines[0]["status"])
}

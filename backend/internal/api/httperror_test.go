package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envelope mirrors api.Response without importing the unexported field tags.
type envelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

func decodeEnvelope(t *testing.T, body string) envelope {
	t.Helper()
	var env envelope
	require.NoError(t, json.Unmarshal([]byte(body), &env), "body was not an API envelope: %s", body)
	return env
}

// Echo's default handler answers with its own {"message":...} shape, which the
// SPA cannot unwrap, so it rendered "undefined" for every framework-level error.
func TestUnroutedPathReturnsEnvelope(t *testing.T) {
	app, _, _ := newTestApp(t, defaultTestConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	env := decodeEnvelope(t, rec.Body.String())
	assert.False(t, env.Success)
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "ERR_FILE_NOT_FOUND", env.Errors[0].Code)
	assert.NotEmpty(t, env.Errors[0].Message)
}

func TestOversizedBodyReturnsEnvelope(t *testing.T) {
	app, _, _ := newTestApp(t, defaultTestConfig())

	// The connect group caps JSON bodies at 1M.
	body := `{"protocol":"ftp","host":"h","port":21,"username":"u","password":"` +
		strings.Repeat("x", 2*1024*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	env := decodeEnvelope(t, rec.Body.String())
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "ERR_FILE_TOO_LARGE", env.Errors[0].Code)
}

// A malformed payload used to be reported as a missing field, telling the caller
// to fix something that was not the problem.
func TestMalformedJSONIsNotReportedAsAMissingField(t *testing.T) {
	app, store, _ := newTestApp(t, defaultTestConfig())
	_ = store

	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect",
		strings.NewReader(`{"protocol": "ftp", `))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	env := decodeEnvelope(t, rec.Body.String())
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "ERR_BAD_REQUEST", env.Errors[0].Code)
	assert.NotContains(t, env.Errors[0].Message, "required",
		"a broken body must not be reported as a validation failure")
}

// The envelope must never carry echo's internal text.
func TestFrameworkErrorsDoNotLeakInternals(t *testing.T) {
	app, _, _ := newTestApp(t, defaultTestConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/nope", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	var top map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &top))
	// echo's own shape is a bare top-level {"message": ...}; ours nests the text
	// inside errors[], which is what the SPA unwraps.
	assert.NotContains(t, top, "message")
	assert.Contains(t, top, "success")
	assert.Contains(t, top, "errors")
}

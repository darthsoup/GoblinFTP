// backend/internal/api/system_test.go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemVarsPublic(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.MaxConcurrentUploads = 4
	app, _, _ := newTestApp(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/system/vars", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Upload struct {
				ChunkSize            int64 `json:"chunkSize"`
				MaxConcurrentUploads int   `json:"maxConcurrentUploads"`
			} `json:"upload"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, cfg.ChunkSize, resp.Data.Upload.ChunkSize)
	assert.Equal(t, cfg.MaxConcurrentUploads, resp.Data.Upload.MaxConcurrentUploads)
}

func TestSystemVarsNoSession(t *testing.T) {
	app, _, _ := newTestApp(t, defaultTestConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/system/vars", nil)
	// No cookie set — should still work (public route)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSystemVarsSSOFields(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.SSOEnabled = true
	cfg.SSOSecret = []byte("testsecret32byteslong_xxxxxxxxxxx")
	cfg.DisableLoginForm = true

	app, _, _ := newTestApp(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/system/vars", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			SSOEnabled        bool `json:"ssoEnabled"`
			LoginFormDisabled bool `json:"loginFormDisabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Data.SSOEnabled)
	assert.True(t, resp.Data.LoginFormDisabled)
}

// backend/internal/api/version_test.go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
)

func TestHealthzReportsVersion(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithVersion("v1.2.3"))
	defer store.Close()

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"ok"`)
	assert.Contains(t, rec.Body.String(), `"version":"v1.2.3"`)
}

func TestHealthzDefaultVersion(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"version":"dev"`)
}

func TestSystemVarsReportsVersion(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithVersion("v1.2.3"))
	defer store.Close()

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/system/vars", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "v1.2.3", resp.Data.Version)
}

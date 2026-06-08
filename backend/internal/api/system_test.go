// backend/internal/api/system_test.go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/config"
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
			Branding struct {
				AppName         string  `json:"appName"`
				LogoURL         *string `json:"logoUrl"`
				HideAttribution bool    `json:"hideAttribution"`
			} `json:"branding"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, cfg.ChunkSize, resp.Data.Upload.ChunkSize)
	assert.Equal(t, cfg.MaxConcurrentUploads, resp.Data.Upload.MaxConcurrentUploads)
	// API returns valid branding defaults (never an empty app name).
	assert.Equal(t, "GoblinFTP", resp.Data.Branding.AppName)
	assert.Nil(t, resp.Data.Branding.LogoURL)
	assert.False(t, resp.Data.Branding.HideAttribution)
}

func TestSystemVarsBranding(t *testing.T) {
	cfg := defaultTestConfig()
	color := "#2563eb"
	logo := "https://acme.example/logo.svg"
	cfg.Settings.Branding = config.BrandingSettings{
		AppName:         "Acme Transfer",
		LogoURL:         &logo,
		PrimaryColor:    &color,
		HideAttribution: true,
	}
	app, _, _ := newTestApp(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/system/vars", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			Branding struct {
				AppName         string  `json:"appName"`
				LogoURL         *string `json:"logoUrl"`
				FaviconURL      *string `json:"faviconUrl"`
				PrimaryColor    *string `json:"primaryColor"`
				HideAttribution bool    `json:"hideAttribution"`
			} `json:"branding"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "Acme Transfer", resp.Data.Branding.AppName)
	require.NotNil(t, resp.Data.Branding.LogoURL)
	assert.Equal(t, logo, *resp.Data.Branding.LogoURL)
	require.NotNil(t, resp.Data.Branding.PrimaryColor)
	assert.Equal(t, color, *resp.Data.Branding.PrimaryColor)
	assert.Nil(t, resp.Data.Branding.FaviconURL)
	assert.True(t, resp.Data.Branding.HideAttribution)
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

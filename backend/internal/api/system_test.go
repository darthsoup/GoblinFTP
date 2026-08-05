// backend/internal/api/system_test.go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	textColor := "#0b1220"
	logo := "https://acme.example/logo.svg"
	cfg.Settings.Branding = config.BrandingSettings{
		AppName:          "Acme Transfer",
		LogoURL:          &logo,
		PrimaryColor:     &color,
		PrimaryTextColor: &textColor,
		HideAttribution:  true,
	}
	app, _, _ := newTestApp(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/system/vars", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			Branding struct {
				AppName          string  `json:"appName"`
				LogoURL          *string `json:"logoUrl"`
				FaviconURL       *string `json:"faviconUrl"`
				PrimaryColor     *string `json:"primaryColor"`
				PrimaryTextColor *string `json:"primaryTextColor"`
				HideAttribution  bool    `json:"hideAttribution"`
			} `json:"branding"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "Acme Transfer", resp.Data.Branding.AppName)
	require.NotNil(t, resp.Data.Branding.LogoURL)
	assert.Equal(t, logo, *resp.Data.Branding.LogoURL)
	require.NotNil(t, resp.Data.Branding.PrimaryColor)
	assert.Equal(t, color, *resp.Data.Branding.PrimaryColor)
	require.NotNil(t, resp.Data.Branding.PrimaryTextColor)
	assert.Equal(t, textColor, *resp.Data.Branding.PrimaryTextColor)
	// No favicon set → falls back to the logo so the tab icon is still branded.
	require.NotNil(t, resp.Data.Branding.FaviconURL)
	assert.Equal(t, logo, *resp.Data.Branding.FaviconURL)
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
	cfg.LoginFormDisabled = true

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

// TestSystemVarsCoversSPAKeys asserts every SPA-flagged registry key exists in
// the /api/system/vars response, so a new key cannot silently miss the frontend.
func TestSystemVarsCoversSPAKeys(t *testing.T) {
	app, _, _ := newTestApp(t, defaultTestConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/system/vars", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	for _, sk := range config.SPAKeys() {
		name := sk.Env
		if name == "" {
			name = sk.JSONPath
		}
		node := resp.Data
		segments := strings.Split(sk.JSONPath, ".")
		for i, seg := range segments {
			raw, ok := node[seg]
			require.True(t, ok, "%s: %q missing at segment %q in /api/system/vars", name, sk.JSONPath, seg)
			if i < len(segments)-1 {
				next := map[string]json.RawMessage{}
				require.NoError(t, json.Unmarshal(raw, &next), "%s: %q is not an object", name, seg)
				node = next
			}
		}
	}
}

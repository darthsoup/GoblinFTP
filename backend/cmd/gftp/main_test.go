// backend/cmd/gftp/main_test.go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Port:                 "8080",
		LogLevel:             "info",
		SessionSecret:        []byte("test-session-secret"),
		DownloadTokenSecret:  []byte("test-download-secret"),
		LoginMaxAttempts:     5,
		LoginCooldownSeconds: 300,
		SessionTTLSeconds:    7200,
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

func TestHealthz(t *testing.T) {
	e := newApp(testConfig())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"ok"`)
}

func TestUnauthenticatedAPIReturns401(t *testing.T) {
	e := newApp(testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

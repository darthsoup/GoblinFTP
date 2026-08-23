package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

// allowAllProtocols permits ftps too; the shared default config allows only
// ftp and sftp.
func allowAllProtocols() *config.Config {
	cfg := defaultTestConfig()
	cfg.Settings.Connection.AllowedTypes = []string{"ftp", "ftps", "sftp"}
	return cfg
}

// chmodSpy records every Chmod call so a test can assert connecting never
// mutates anything on the server.
type chmodSpy struct {
	mu    sync.Mutex
	calls []string
}

func (s *chmodSpy) record(p string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, p)
}

func (s *chmodSpy) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

func connectWith(t *testing.T, app http.Handler, protocol string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"protocol":"` + protocol + `","host":"h.example.com","port":21,"username":"u","password":"p"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func capabilityFrom(t *testing.T, rec *httptest.ResponseRecorder) bool {
	t.Helper()
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Capabilities struct {
				DisableChmod bool `json:"disableChmod"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success, "body: %s", rec.Body.String())
	return resp.Data.Capabilities.DisableChmod
}

// Regression: a probe used to chmod the session's working directory to 0755 on
// connect, silently widening a private 0700 home the login user owned.
func TestConnectDoesNotChmodWorkingDirectory(t *testing.T) {
	for _, protocol := range []string{"ftp", "ftps", "sftp"} {
		t.Run(protocol, func(t *testing.T) {
			spy := &chmodSpy{}
			mock := &testutil.MockClient{
				WorkingDirFn: func() (string, error) { return "/home/user", nil },
				ChmodFn: func(p string, _ uint32) error {
					spy.record(p)
					return nil
				},
			}
			app, _, _ := newTestApp(t, allowAllProtocols(), api.WithDial(staticDial(mock)))

			rec := connectWith(t, app, protocol)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
			assert.Empty(t, spy.seen(), "connecting must not modify permissions on the server")
		})
	}
}

// The SSO path carried the same probe and connects on every page load, so it
// needs its own guard.
func TestSSOConnectDoesNotChmodWorkingDirectory(t *testing.T) {
	spy := &chmodSpy{}
	cfg := ssoEnabledConfig()
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/home/user", nil },
		ChmodFn: func(p string, _ uint32) error {
			spy.record(p)
			return nil
		},
	}
	e, store, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
	defer store.Close()

	cookie, csrf := establishSSO(t, e, validSSOSFTP(t, cfg.SSOSecret))
	rec := doSSOConnect(e, cookie, csrf, "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Empty(t, spy.seen(), "SSO connect must not modify permissions on the server")
}

// The advertised capability must match what the adapter can actually do, or the
// UI offers an action that can only fail.
func TestConnectReportsChmodCapabilityPerProtocol(t *testing.T) {
	tests := []struct {
		protocol     string
		supports     bool
		wantDisabled bool
	}{
		{"ftp", false, true},
		{"ftps", false, true},
		{"sftp", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.protocol, func(t *testing.T) {
			mock := &testutil.MockClient{
				WorkingDirFn:    func() (string, error) { return "/home/user", nil },
				SupportsChmodFn: func() bool { return tc.supports },
			}
			app, _, _ := newTestApp(t, allowAllProtocols(), api.WithDial(staticDial(mock)))

			rec := connectWith(t, app, tc.protocol)
			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
			assert.Equal(t, tc.wantDisabled, capabilityFrom(t, rec))
		})
	}
}

// A page reload must see the same capability: the UI gates the chmod control on it.
func TestChmodCapabilitySurvivesStatusRestore(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn:    func() (string, error) { return "/home/user", nil },
		SupportsChmodFn: func() bool { return false },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))

	rec := connectWith(t, app, "ftp")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, capabilityFrom(t, rec))

	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	for _, ck := range rec.Result().Cookies() {
		statusReq.AddCookie(ck)
	}
	statusRec := httptest.NewRecorder()
	app.ServeHTTP(statusRec, statusReq)
	require.Equal(t, http.StatusOK, statusRec.Code)

	var resp struct {
		Data struct {
			Connected    bool `json:"connected"`
			Capabilities *struct {
				DisableChmod bool `json:"disableChmod"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(statusRec.Body.Bytes(), &resp))
	require.True(t, resp.Data.Connected)
	require.NotNil(t, resp.Data.Capabilities)
	assert.True(t, resp.Data.Capabilities.DisableChmod)
}

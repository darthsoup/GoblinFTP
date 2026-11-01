package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/sso"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestConnectRejectsWindowsWorkingDirectories(t *testing.T) {
	for _, protocol := range []string{"ftp", "ftps", "sftp"} {
		for _, directory := range []string{`\`, `C:\Users\test`, `C:/Users/test`, `\\server\share`} {
			t.Run(protocol+directory, func(t *testing.T) {
				closed := false
				mock := workingMock()
				mock.WorkingDirFn = func() (string, error) { return directory, nil }
				mock.CloseFn = func() error { closed = true; return nil }
				cfg := defaultTestConfig()
				cfg.Settings.Connection.AllowedTypes = []string{"ftp", "ftps", "sftp"}
				app, store, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
				payload := validPayload()
				payload.Protocol = protocol
				rec := httptest.NewRecorder()
				app.ServeHTTP(rec, newConnectRequest(t, payload))
				require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
				assert.Contains(t, rec.Body.String(), "ERR_UNSUPPORTED_SERVER")
				assert.True(t, closed)
				assert.Zero(t, store.Count())
				assert.Empty(t, rec.Result().Cookies())
			})
		}
	}
}

func TestUnsupportedConnectionPreservesExistingSession(t *testing.T) {
	active := workingMock()
	rejected := workingMock()
	activeClosed, rejectedClosed := false, false
	active.CloseFn = func() error { activeClosed = true; return nil }
	rejected.CloseFn = func() error { rejectedClosed = true; return nil }
	rejected.WorkingDirFn = func() (string, error) { return `\`, nil }
	calls := 0
	app, store, _ := newTestApp(t, defaultTestConfig(), api.WithDial(func(api.DialRequest) (transfer.Client, *api.HostKeyPrompt, error) {
		calls++
		if calls == 1 {
			return active, nil, nil
		}
		return rejected, nil, nil
	}))
	sess := connectAndGetSession(t, app)
	req := newConnectRequest(t, validPayload())
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.True(t, rejectedClosed)
	assert.False(t, activeClosed)
	assert.Equal(t, 1, store.Count())
	assert.Empty(t, rec.Result().Cookies())
	status := doJSON(app, sess, http.MethodGet, "/api/auth/status", "")
	assert.Contains(t, status.Body.String(), `"connected":true`)
}

func TestSSORejectsWindowsWorkingDirectoryBeforeInitialDirectoryOverride(t *testing.T) {
	cfg := ssoEnabledConfig()
	closed := false
	mock := workingMock()
	mock.WorkingDirFn = func() (string, error) { return `C:\Files`, nil }
	mock.CloseFn = func() error { closed = true; return nil }
	mock.StatFn = func(string) (transfer.FileInfo, error) {
		t.Fatal("unsupported server reached remote stat")
		return transfer.FileInfo{}, nil
	}
	app, store, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
	token, err := sso.Encrypt(&sso.Payload{Type: "ftp", Host: "ftp.example.com", Port: 21, Username: "user", Password: "pass", InitialDirectory: "/", Exp: time.Now().Add(time.Minute).Unix()}, cfg.SSOSecret)
	require.NoError(t, err)
	login := httptest.NewRecorder()
	app.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/?sso="+token, nil))
	require.Equal(t, http.StatusFound, login.Code)
	var pending *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == "gftp_sso" && cookie.Value != "" && cookie.MaxAge >= 0 {
			pending = cookie
			break
		}
	}
	require.NotNil(t, pending)
	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/sso-pending", nil)
	statusReq.AddCookie(pending)
	status := httptest.NewRecorder()
	app.ServeHTTP(status, statusReq)
	var response struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(status.Body.Bytes(), &response))
	require.NotEmpty(t, response.Data.CSRFToken)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sso-connect", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.CSRFHeaderName, response.Data.CSRFToken)
	req.AddCookie(pending)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "ERR_UNSUPPORTED_SERVER")
	assert.True(t, closed)
	assert.Equal(t, 1, store.Count())
	assert.Empty(t, rec.Result().Cookies())
}

func TestFTPListingRejectsWindowsPathsBeforeRemoteOperation(t *testing.T) {
	mock := workingMock()
	mock.ListFn = func(string) ([]transfer.FileInfo, error) {
		t.Fatal("unsupported path reached remote listing")
		return nil, nil
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)
	for _, p := range []string{`\`, `\/file`, `C:/Files`, `/C:/Files`, `/dir\name`, "/dir.", "/dir "} {
		rec := doJSON(app, sess, http.MethodGet, "/api/files?path="+url.QueryEscape(p), "")
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestSFTPPreservesPOSIXBackslashFilenames(t *testing.T) {
	mock := workingMock()
	mock.WorkingDirFn = func() (string, error) { return `/home/user\name`, nil }
	mock.ListFn = func(p string) ([]transfer.FileInfo, error) { assert.Equal(t, `/home/user\name`, p); return nil, nil }
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	payload := validPayload()
	payload.Protocol = "sftp"
	connected := httptest.NewRecorder()
	app.ServeHTTP(connected, newConnectRequest(t, payload))
	require.Equal(t, http.StatusOK, connected.Code, connected.Body.String())
	req := httptest.NewRequest(http.MethodGet, "/api/files?path="+url.QueryEscape(`/home/user\name`), nil)
	for _, cookie := range connected.Result().Cookies() {
		if cookie.MaxAge >= 0 && cookie.Value != "" {
			req.AddCookie(cookie)
		}
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

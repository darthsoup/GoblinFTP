// backend/internal/api/connect_test.go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

type connectPayload struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func marshalPayload(t *testing.T, p connectPayload) string {
	t.Helper()
	b, err := json.Marshal(p)
	require.NoError(t, err)
	return string(b)
}

func newConnectRequest(t *testing.T, p connectPayload) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect", strings.NewReader(marshalPayload(t, p)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func validPayload() connectPayload {
	return connectPayload{Protocol: "ftp", Host: "ftp.example.com", Port: 21, Username: "user", Password: "pass"}
}

func TestConnectDisallowedType(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Settings.Connection.AllowedTypes = []string{"ftp"} // sftp not allowed
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	p := validPayload()
	p.Protocol = "sftp"
	req := newConnectRequest(t, p)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, string(gftperrors.ErrInvalidType), resp.Errors[0].Code)
}

func TestConnectMissingHost(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	p := validPayload()
	p.Host = ""
	req := newConnectRequest(t, p)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConnectMissingUsername(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	p := validPayload()
	p.Username = ""
	req := newConnectRequest(t, p)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConnectInvalidPort(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	p := validPayload()
	p.Port = 0
	req := newConnectRequest(t, p)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConnectIPNotAllowlisted(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Settings.Access.AllowedClientAddresses = []string{"10.0.0.1"} // only 10.0.0.1 allowed
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	req := newConnectRequest(t, validPayload())
	req.RemoteAddr = "192.168.1.100:12345" // not in allowlist
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, string(gftperrors.ErrForbidden), resp.Errors[0].Code)
}

func TestConnectThrottled(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.LoginMaxAttempts = 2
	e, store, throttle := newTestApp(t, cfg)
	defer store.Close()

	key := "ftp.example.com:user"
	throttle.Record(key, 1*time.Minute)
	throttle.Record(key, 1*time.Minute)

	req := newConnectRequest(t, validPayload())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, string(gftperrors.ErrLoginThrottled), resp.Errors[0].Code)
}

func TestConnectSuccess(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/home/user", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
	}
	dialFn := func(protocol, addr, user, pass string, passive bool) (transfer.Client, error) {
		return mock, nil
	}

	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	body := `{"protocol":"ftp","host":"ftp.example.com","port":21,"username":"user","password":"pass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			InitialDirectory string `json:"initialDirectory"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "/home/user", resp.Data.InitialDirectory)
}

func TestDisconnect(t *testing.T) {
	e, store, _ := newTestApp(t, defaultTestConfig())
	defer store.Close()

	sess, err := store.New()
	require.NoError(t, err)
	csrfToken, err := auth.GenerateCSRFToken()
	require.NoError(t, err)
	sess.Set(auth.CSRFSessionKey, csrfToken)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/disconnect", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.CSRFHeaderName, csrfToken)
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Session must be deleted
	_, ok := store.Get(sess.ID)
	assert.False(t, ok, "session should be deleted after disconnect")

	// Cookie must be cleared (MaxAge = -1)
	var found bool
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == api.SessionCookieName {
			found = true
			assert.Equal(t, -1, cookie.MaxAge)
		}
	}
	assert.True(t, found, "session cookie should be present in response to clear it")
}

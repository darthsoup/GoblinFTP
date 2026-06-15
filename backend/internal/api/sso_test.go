// backend/internal/api/sso_test.go
package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/sso"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func ssoEnabledConfig() *config.Config {
	cfg := defaultTestConfig()
	cfg.SSOEnabled = true
	cfg.SSOSecret = []byte("test-sso-secret-32bytes-xxxxxxxxxxx")
	return cfg
}

// validSSO creates an encrypted SSO token with future expiry.
func validSSO(t *testing.T, secret []byte) string {
	t.Helper()
	payload := &sso.Payload{
		Type:     "ftp",
		Host:     "ftp.example.com",
		Port:     21,
		Username: "user",
		Password: "pass",
		Exp:      time.Now().Add(5 * time.Minute).Unix(),
	}
	tok, err := sso.Encrypt(payload, secret)
	require.NoError(t, err)
	return tok
}

func TestSSOLoginNoParam(t *testing.T) {
	cfg := ssoEnabledConfig()
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "GoblinFTP", rec.Body.String())
}

func TestSSOLoginDisabled(t *testing.T) {
	cfg := defaultTestConfig() // SSO disabled
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/?sso=anytoken", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/login?sso_error=disabled", rec.Header().Get("Location"))
}

func TestSSOLoginInvalidToken(t *testing.T) {
	cfg := ssoEnabledConfig()
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/?sso=garbage-token", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/login?sso_error=invalid", rec.Header().Get("Location"))
}

func TestSSOLoginExpiredToken(t *testing.T) {
	cfg := ssoEnabledConfig()
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	// Create an expired token
	payload := &sso.Payload{
		Type:     "ftp",
		Host:     "ftp.example.com",
		Port:     21,
		Username: "user",
		Password: "pass",
		Exp:      time.Now().Add(-5 * time.Minute).Unix(), // expired
	}
	tok, err := sso.Encrypt(payload, cfg.SSOSecret)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/?sso="+tok, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/login?sso_error=expired", rec.Header().Get("Location"))
}

func TestSSOLoginSuccess(t *testing.T) {
	cfg := ssoEnabledConfig()
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	tok := validSSO(t, cfg.SSOSecret)

	req := httptest.NewRequest(http.MethodGet, "/?sso="+tok, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))

	// Check that a session cookie was set
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == api.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie, "expected gftp_session cookie to be set")
	assert.NotEmpty(t, sessionCookie.Value)
}

func TestSSOLoginReplay(t *testing.T) {
	cfg := ssoEnabledConfig()
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	tok := validSSO(t, cfg.SSOSecret)

	// First attempt should succeed
	req1 := httptest.NewRequest(http.MethodGet, "/?sso="+tok, nil)
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusFound, rec1.Code)

	// Second attempt with same token should fail
	req2 := httptest.NewRequest(http.MethodGet, "/?sso="+tok, nil)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusFound, rec2.Code)
	assert.Equal(t, "/login?sso_error=used", rec2.Header().Get("Location"))
}

func TestAuthStatusNoSession(t *testing.T) {
	cfg := ssoEnabledConfig()
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Connected      bool   `json:"connected"`
			SSOAutoConnect bool   `json:"ssoAutoConnect"`
			CSRFToken      string `json:"csrfToken"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.False(t, resp.Data.Connected)
	assert.False(t, resp.Data.SSOAutoConnect)
	assert.Empty(t, resp.Data.CSRFToken)
}

func TestAuthStatusWithSSOPending(t *testing.T) {
	cfg := ssoEnabledConfig()
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	tok := validSSO(t, cfg.SSOSecret)

	// Hit SSOLogin to create session with pending SSO
	loginReq := httptest.NewRequest(http.MethodGet, "/?sso="+tok, nil)
	loginRec := httptest.NewRecorder()
	e.ServeHTTP(loginRec, loginReq)
	require.Equal(t, http.StatusFound, loginRec.Code)

	// Get the session cookie
	cookies := loginRec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == api.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie)

	// Now check auth status with that cookie
	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusReq.AddCookie(sessionCookie)
	statusRec := httptest.NewRecorder()
	e.ServeHTTP(statusRec, statusReq)

	assert.Equal(t, http.StatusOK, statusRec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Connected      bool   `json:"connected"`
			SSOAutoConnect bool   `json:"ssoAutoConnect"`
			CSRFToken      string `json:"csrfToken"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(statusRec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.False(t, resp.Data.Connected)
	assert.True(t, resp.Data.SSOAutoConnect)
	assert.NotEmpty(t, resp.Data.CSRFToken)
}

func TestAuthStatusConnected(t *testing.T) {
	cfg := defaultTestConfig()
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/home/user", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
	}
	dialFn := staticDial(mock)
	e, store, _ := newTestApp(t, cfg, api.WithDial(dialFn))
	defer store.Close()

	// Connect via regular auth
	sess := connectAndGetSession(t, e)

	// Check auth status
	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	addSession(statusReq, sess)
	statusRec := httptest.NewRecorder()
	e.ServeHTTP(statusRec, statusReq)

	assert.Equal(t, http.StatusOK, statusRec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Connected        bool   `json:"connected"`
			SSOAutoConnect   bool   `json:"ssoAutoConnect"`
			CSRFToken        string `json:"csrfToken"`
			Host             string `json:"host"`
			InitialDirectory string `json:"initialDirectory"`
			Capabilities     *struct {
				DisableChmod bool `json:"disableChmod"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(statusRec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.True(t, resp.Data.Connected)
	assert.False(t, resp.Data.SSOAutoConnect)
	assert.NotEmpty(t, resp.Data.CSRFToken)
	// Connection context is returned so the SPA can restore state after a reload.
	assert.NotEmpty(t, resp.Data.Host)
	assert.Equal(t, "/home/user", resp.Data.InitialDirectory)
	require.NotNil(t, resp.Data.Capabilities)
	assert.False(t, resp.Data.Capabilities.DisableChmod)
}

func TestSSOConnectNoPending(t *testing.T) {
	cfg := ssoEnabledConfig()
	e, store, _ := newTestApp(t, cfg)
	defer store.Close()

	// Create a plain session without sso_pending
	sess, err := store.New()
	require.NoError(t, err)
	csrfToken, err := auth.GenerateCSRFToken()
	require.NoError(t, err)
	sess.Set(auth.CSRFSessionKey, csrfToken)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/sso-connect", nil)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: sess.ID})
	req.Header.Set(auth.CSRFHeaderName, csrfToken)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, string(gftperrors.ErrUnauthorized), resp.Errors[0].Code)
}

func TestSSOConnectFullFlow(t *testing.T) {
	cfg := ssoEnabledConfig()
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/home/user", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
	}
	dialFn := staticDial(mock)
	e, store, _ := newTestApp(t, cfg, api.WithDial(dialFn))
	defer store.Close()

	tok := validSSO(t, cfg.SSOSecret)

	// Step 1: SSOLogin
	loginReq := httptest.NewRequest(http.MethodGet, "/?sso="+tok, nil)
	loginRec := httptest.NewRecorder()
	e.ServeHTTP(loginRec, loginReq)
	require.Equal(t, http.StatusFound, loginRec.Code)

	// Get session cookie
	cookies := loginRec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == api.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie)

	// Step 2: Get auth status to retrieve CSRF token
	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusReq.AddCookie(sessionCookie)
	statusRec := httptest.NewRecorder()
	e.ServeHTTP(statusRec, statusReq)
	require.Equal(t, http.StatusOK, statusRec.Code)

	var statusResp struct {
		Data struct {
			SSOAutoConnect bool   `json:"ssoAutoConnect"`
			CSRFToken      string `json:"csrfToken"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(statusRec.Body.Bytes(), &statusResp))
	require.True(t, statusResp.Data.SSOAutoConnect)
	require.NotEmpty(t, statusResp.Data.CSRFToken)

	// Step 3: SSOConnect
	connectReq := httptest.NewRequest(http.MethodPost, "/api/auth/sso-connect", nil)
	connectReq.Header.Set("Content-Type", "application/json")
	connectReq.AddCookie(sessionCookie)
	connectReq.Header.Set(auth.CSRFHeaderName, statusResp.Data.CSRFToken)
	connectRec := httptest.NewRecorder()
	e.ServeHTTP(connectRec, connectReq)

	assert.Equal(t, http.StatusOK, connectRec.Code)
	var connectResp struct {
		Success bool `json:"success"`
		Data    struct {
			Capabilities struct {
				DisableChmod bool `json:"disableChmod"`
			} `json:"capabilities"`
			InitialDirectory string `json:"initialDirectory"`
			CSRFToken        string `json:"csrfToken"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(connectRec.Body.Bytes(), &connectResp))
	assert.True(t, connectResp.Success)
	assert.Equal(t, "/home/user", connectResp.Data.InitialDirectory)
	assert.NotEmpty(t, connectResp.Data.CSRFToken)
}

// validSSOSFTP creates an encrypted SFTP SSO token with future expiry — the
// host-key flow only applies to sftp.
func validSSOSFTP(t *testing.T, secret []byte) string {
	t.Helper()
	payload := &sso.Payload{
		Type:     "sftp",
		Host:     "ssh.example.com",
		Port:     22,
		Username: "user",
		Password: "pass",
		Exp:      time.Now().Add(5 * time.Minute).Unix(),
	}
	tok, err := sso.Encrypt(payload, secret)
	require.NoError(t, err)
	return tok
}

// establishSSO runs the SSO login + status handshake and returns the session
// cookie and CSRF token needed to POST /api/auth/sso-connect.
func establishSSO(t *testing.T, e http.Handler, tok string) (*http.Cookie, string) {
	t.Helper()
	loginRec := httptest.NewRecorder()
	e.ServeHTTP(loginRec, httptest.NewRequest(http.MethodGet, "/?sso="+tok, nil))
	require.Equal(t, http.StatusFound, loginRec.Code)

	var sessionCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == api.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusReq.AddCookie(sessionCookie)
	statusRec := httptest.NewRecorder()
	e.ServeHTTP(statusRec, statusReq)
	require.Equal(t, http.StatusOK, statusRec.Code)

	var statusResp struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(statusRec.Body.Bytes(), &statusResp))
	require.NotEmpty(t, statusResp.Data.CSRFToken)
	return sessionCookie, statusResp.Data.CSRFToken
}

// doSSOConnect POSTs /api/auth/sso-connect with the given session/CSRF and body
// (empty body string → no JSON body).
func doSSOConnect(e http.Handler, cookie *http.Cookie, csrf, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/sso-connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	req.Header.Set(auth.CSRFHeaderName, csrf)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// TestSSOConnectHostKeyPrompt: an unknown SFTP host key returns a 200 with the
// fingerprint (and no client pinned to the session); confirming it with a second
// request that carries the fingerprint completes the connection. The pending SSO
// request survives the prompt so the retry can use it.
func TestSSOConnectHostKeyPrompt(t *testing.T) {
	cfg := ssoEnabledConfig()
	mock := workingMock()
	calls := 0
	dialFn := api.WithDial(func(req api.DialRequest) (transfer.Client, *api.HostKeyPrompt, error) {
		calls++
		if req.AcceptHostKey == "" {
			return nil, &api.HostKeyPrompt{Host: req.Host, Fingerprint: "SHA256:abc123", KeyType: "ssh-ed25519"}, nil
		}
		return mock, nil, nil
	})
	e, store, _ := newTestApp(t, cfg, dialFn)
	defer store.Close()

	cookie, csrf := establishSSO(t, e, validSSOSFTP(t, cfg.SSOSecret))

	// First attempt → host-key prompt, no session client yet.
	rec := doSSOConnect(e, cookie, csrf, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			HostKeyPrompt *struct {
				Host        string `json:"host"`
				Fingerprint string `json:"fingerprint"`
				KeyType     string `json:"keyType"`
			} `json:"hostKeyPrompt"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.HostKeyPrompt)
	assert.Equal(t, "SHA256:abc123", resp.Data.HostKeyPrompt.Fingerprint)
	assert.Equal(t, "ssh-ed25519", resp.Data.HostKeyPrompt.KeyType)
	assert.Equal(t, "ssh.example.com", resp.Data.HostKeyPrompt.Host)

	sess, ok := store.Get(cookie.Value)
	require.True(t, ok)
	_, hasClient := sess.Get("client")
	assert.False(t, hasClient, "no client until the host key is confirmed")

	// Second attempt with the trusted fingerprint → connection proceeds.
	rec2 := doSSOConnect(e, cookie, csrf, `{"acceptHostKey":"SHA256:abc123"}`)
	require.Equal(t, http.StatusOK, rec2.Code)
	var resp2 struct {
		Success bool `json:"success"`
		Data    struct {
			InitialDirectory string `json:"initialDirectory"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))
	assert.True(t, resp2.Success)
	assert.Equal(t, "/", resp2.Data.InitialDirectory)
	assert.Equal(t, 2, calls, "dial called once for the prompt and once for the confirmed retry")
}

// TestSSOConnectHostKeyMismatch: a changed host key is refused with
// ERR_HOST_KEY_MISMATCH and the raw cause is not leaked into the envelope.
func TestSSOConnectHostKeyMismatch(t *testing.T) {
	cfg := ssoEnabledConfig()
	dialFn := api.WithDial(func(api.DialRequest) (transfer.Client, *api.HostKeyPrompt, error) {
		return nil, nil, fmt.Errorf("%w: raw-detail-xyz", transfer.ErrHostKeyMismatch)
	})
	e, store, _ := newTestApp(t, cfg, dialFn)
	defer store.Close()

	cookie, csrf := establishSSO(t, e, validSSOSFTP(t, cfg.SSOSecret))

	rec := doSSOConnect(e, cookie, csrf, "")
	require.Equal(t, http.StatusBadGateway, rec.Code)
	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	require.NotEmpty(t, resp.Errors)
	assert.Equal(t, string(gftperrors.ErrHostKeyMismatch), resp.Errors[0].Code)
	assert.NotContains(t, resp.Errors[0].Message, "raw-detail-xyz", "raw cause must not leak")
}

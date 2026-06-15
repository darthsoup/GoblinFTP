// backend/internal/api/status_ping_test.go
package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

type statusResp struct {
	Success bool `json:"success"`
	Data    struct {
		Connected      bool   `json:"connected"`
		SSOAutoConnect bool   `json:"ssoAutoConnect"`
		CSRFToken      string `json:"csrfToken"`
	} `json:"data"`
}

func getStatus(t *testing.T, e http.Handler, sess sessionCtx, query string) statusResp {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status"+query, nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp statusResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func TestAuthStatusPingAlive(t *testing.T) {
	cfg := defaultTestConfig()
	pinged := false
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
		PingFn:       func() error { pinged = true; return nil },
	}
	e, store, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
	defer store.Close()
	sess := connectAndGetSession(t, e)

	resp := getStatus(t, e, sess, "?ping=1")
	assert.True(t, resp.Data.Connected)
	assert.True(t, pinged)
	assert.False(t, mock.Closed)
}

func TestAuthStatusPingDead(t *testing.T) {
	cfg := defaultTestConfig()
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
		PingFn:       func() error { return errors.New("connection reset") },
	}
	e, store, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
	defer store.Close()
	sess := connectAndGetSession(t, e)

	resp := getStatus(t, e, sess, "?ping=1")
	assert.False(t, resp.Data.Connected, "dead connection must report connected=false")
	assert.True(t, mock.Closed, "dead client must be closed")
	// CSRF token survives so the SPA can still talk to the session.
	assert.NotEmpty(t, resp.Data.CSRFToken)

	// The client is gone from the session: a plain status check agrees.
	resp = getStatus(t, e, sess, "")
	assert.False(t, resp.Data.Connected)
}

func TestAuthStatusWithoutPingDoesNotPing(t *testing.T) {
	cfg := defaultTestConfig()
	pinged := false
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
		PingFn:       func() error { pinged = true; return nil },
	}
	e, store, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
	defer store.Close()
	sess := connectAndGetSession(t, e)

	resp := getStatus(t, e, sess, "")
	assert.True(t, resp.Data.Connected)
	assert.False(t, pinged, "plain status must not ping the server")
}

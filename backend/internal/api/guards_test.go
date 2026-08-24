package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

// Stat("/") short-circuits to a directory, so Delete("/") routed straight into
// RemoveDirRecur("/") and wiped the account, then reported one failed path.
func TestDeleteRefusesRemoteRoot(t *testing.T) {
	for _, target := range []string{"/", "//", "/.", " / "} {
		t.Run(target, func(t *testing.T) {
			var deleted []string
			mock := &testutil.MockClient{
				WorkingDirFn: func() (string, error) { return "/", nil },
				DeleteFn:     func(p string) error { deleted = append(deleted, p); return nil },
			}
			app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
			sess := connectAndGetSession(t, app)

			req := httptest.NewRequest(http.MethodDelete, "/api/files",
				strings.NewReader(`{"paths":["`+target+`"]}`))
			req.Header.Set("Content-Type", "application/json")
			addSession(req, sess)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, deleted, "Delete must never be reached for the remote root")
		})
	}
}

// The whole batch is refused, so a root entry cannot take its siblings with it.
func TestDeleteRefusesBatchContainingRoot(t *testing.T) {
	var deleted []string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		DeleteFn:     func(p string) error { deleted = append(deleted, p); return nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodDelete, "/api/files",
		strings.NewReader(`{"paths":["/safe.txt","/"]}`))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, deleted)
}

// GFTP_LOGIN_FORM_DISABLED only hid the SPA form, so an SSO-only instance stayed
// an unauthenticated dialer for arbitrary hosts over a direct POST.
func TestLoginDisabledRejectsManualConnect(t *testing.T) {
	dialed := false
	mock := &testutil.MockClient{WorkingDirFn: func() (string, error) { return "/", nil }}
	cfg := defaultTestConfig()
	cfg.LoginFormDisabled = true

	app, _, _ := newTestApp(t, cfg, api.WithDial(func(api.DialRequest) (transfer.Client, *api.HostKeyPrompt, error) {
		dialed = true
		return mock, nil, nil
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect",
		strings.NewReader(`{"protocol":"ftp","host":"internal.host","port":21,"username":"u","password":"p"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_LOGIN_DISABLED")
	assert.False(t, dialed, "a disabled login must not reach the dialer at all")
}

func TestLoginEnabledStillConnects(t *testing.T) {
	mock := &testutil.MockClient{WorkingDirFn: func() (string, error) { return "/", nil }}
	cfg := defaultTestConfig()
	cfg.LoginFormDisabled = false

	app, _, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))

	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect",
		strings.NewReader(`{"protocol":"ftp","host":"h","port":21,"username":"u","password":"p"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// An explicit disconnect must release staged chunks, not leave them for the
// 24 hour sweeper.
func TestDisconnectReleasesStagedChunks(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		StatFn:       func(string) (transfer.FileInfo, error) { return transfer.FileInfo{}, transfer.ErrNotFound },
	}
	store := newMemChunkStore()
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)), api.WithChunkStore(store))
	sess := connectAndGetSession(t, app)

	reserveBody := `{"path":"/big.bin","totalChunks":2,"totalSize":10,"chunkSize":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/upload/reserve", strings.NewReader(reserveBody))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "reserve: %s", rec.Body.String())

	dc := httptest.NewRequest(http.MethodPost, "/api/auth/disconnect", nil)
	addSession(dc, sess)
	dcRec := httptest.NewRecorder()
	app.ServeHTTP(dcRec, dc)
	require.Equal(t, http.StatusOK, dcRec.Code)

	assert.NotEmpty(t, store.cleaned, "disconnecting must discard the reserved upload's chunks")
}

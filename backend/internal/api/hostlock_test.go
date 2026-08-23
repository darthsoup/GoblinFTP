// backend/internal/api/hostlock_test.go
package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/config"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

// lockedHostConfig pins the instance to preset.example:21.
func lockedHostConfig(t *testing.T, withPort bool) *config.Config {
	t.Helper()
	cfg := defaultTestConfig()
	host := "preset.example"
	cfg.Settings.Connection.PresetHost = &host
	cfg.Settings.Connection.LockHost = true
	if withPort {
		port := 21
		cfg.Settings.Connection.PresetPort = &port
	}
	return cfg
}

// TestConnectHostLockEnforcedServerSide is the regression test for a policy
// bypass: GFTP_CONNECTION_LOCK_HOST only ever disabled the SPA's host input, so
// a direct POST could dial anywhere. That defeated the operator's lock and gave
// an unauthenticated caller a way to probe the internal network, since the
// error codes distinguish "refused" from "auth failed".
func TestConnectHostLockEnforcedServerSide(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		port      int
		withPort  bool
		wantCode  int
		wantError gftperrors.Code
	}{
		{"preset host is allowed", "preset.example", 21, false, http.StatusOK, ""},
		{"different host is refused", "10.0.0.5", 21, false, http.StatusForbidden, gftperrors.ErrForbidden},
		{"case and trailing dot still match", "PRESET.EXAMPLE.", 21, false, http.StatusOK, ""},
		{"different port refused when pinned", "preset.example", 2121, true, http.StatusForbidden, gftperrors.ErrForbidden},
		{"any port allowed when not pinned", "preset.example", 2121, false, http.StatusOK, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &testutil.MockClient{
				WorkingDirFn: func() (string, error) { return "/", nil },
			}
			app, _, _ := newTestApp(t, lockedHostConfig(t, tc.withPort), api.WithDial(staticDial(mock)))

			req := newConnectRequest(t, connectPayload{
				Protocol: "ftp", Host: tc.host, Port: tc.port,
				Username: "user", Password: "pass",
			})
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantCode, rec.Code, "body: %s", rec.Body.String())
			if tc.wantError != "" {
				assert.Contains(t, rec.Body.String(), string(tc.wantError))
			}
		})
	}
}

// TestConnectHostLockOffAllowsAnyHost guards against the check firing when the
// operator never asked for a lock - the default deployment.
func TestConnectHostLockOffAllowsAnyHost(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))

	req := newConnectRequest(t, connectPayload{
		Protocol: "ftp", Host: "anywhere.example", Port: 21,
		Username: "user", Password: "pass",
	})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

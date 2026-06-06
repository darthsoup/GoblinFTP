// backend/internal/api/connlost_test.go
package api_test

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

// TestListConnLost: a dead FTP connection during list must not leak the raw
// socket error — it maps to ERR_CONNECTION_LOST, the client is dropped from
// the session, and a follow-up status reports connected=false.
func TestListConnLost(t *testing.T) {
	cfg := defaultTestConfig()
	brokenPipe := &net.OpError{Op: "write", Net: "tcp", Err: syscall.EPIPE}
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
		ListFn:       func(path string) ([]transfer.FileInfo, error) { return nil, brokenPipe },
	}
	e, store, _ := newTestApp(t, cfg, api.WithDial(func(protocol, addr, user, pass string, passive bool) (transfer.Client, error) {
		return mock, nil
	}))
	defer store.Close()
	sess := connectAndGetSession(t, e)

	req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.Equal(t, string(gftperrors.ErrConnectionLost), resp.Errors[0].Code)
	assert.NotContains(t, resp.Errors[0].Message, "broken pipe", "raw socket error must not leak")
	assert.True(t, mock.Closed, "dead client must be closed")

	// Session survives, but the client is gone.
	status := getStatus(t, e, sess, "")
	assert.False(t, status.Data.Connected)
	assert.NotEmpty(t, status.Data.CSRFToken)
}

// TestListOtherErrorKeepsClient: non-connection errors keep the original
// code/message and leave the connection in place.
func TestListOtherErrorKeepsClient(t *testing.T) {
	cfg := defaultTestConfig()
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
		ListFn:       func(path string) ([]transfer.FileInfo, error) { return nil, errors.New("550 permission denied") },
	}
	e, store, _ := newTestApp(t, cfg, api.WithDial(func(protocol, addr, user, pass string, passive bool) (transfer.Client, error) {
		return mock, nil
	}))
	defer store.Close()
	sess := connectAndGetSession(t, e)

	req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, string(gftperrors.ErrListFailed), resp.Errors[0].Code)
	assert.Contains(t, resp.Errors[0].Message, "permission denied")
	assert.False(t, mock.Closed)

	status := getStatus(t, e, sess, "")
	assert.True(t, status.Data.Connected)
}

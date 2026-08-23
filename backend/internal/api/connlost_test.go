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

// A dead connection must map to ERR_CONNECTION_LOST without leaking the raw
// socket error, and the client must be dropped from the surviving session.
func TestListConnLost(t *testing.T) {
	cfg := defaultTestConfig()
	brokenPipe := &net.OpError{Op: "write", Net: "tcp", Err: syscall.EPIPE}
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
		ListFn:       func(path string) ([]transfer.FileInfo, error) { return nil, brokenPipe },
	}
	e, store, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
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
	assert.True(t, mock.IsClosed(), "dead client must be closed")

	status := getStatus(t, e, sess, "")
	assert.False(t, status.Data.Connected)
	assert.NotEmpty(t, status.Data.CSRFToken)
}

// A non-connection error keeps the connection and is classified into a friendly
// code plus message. The raw "550 ..." string must not reach the envelope.
func TestListOtherErrorKeepsClient(t *testing.T) {
	cfg := defaultTestConfig()
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
		ListFn:       func(path string) ([]transfer.FileInfo, error) { return nil, errors.New("550 permission denied") },
	}
	e, store, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
	defer store.Close()
	sess := connectAndGetSession(t, e)

	req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, string(gftperrors.ErrFilePermission), resp.Errors[0].Code)
	assert.Contains(t, resp.Errors[0].Message, "Permission denied")
	assert.NotContains(t, resp.Errors[0].Message, "550", "raw protocol string must not leak")
	assert.False(t, mock.IsClosed())

	status := getStatus(t, e, sess, "")
	assert.True(t, status.Data.Connected)
}

// An unclassifiable error keeps the caller's category code (ERR_LIST_FAILED) but
// still surfaces a friendly message instead of the raw error string.
func TestListGenericErrorKeepsCode(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
		ListFn:       func(path string) ([]transfer.FileInfo, error) { return nil, errors.New("weird unexpected error") },
	}
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	defer store.Close()
	sess := connectAndGetSession(t, e)

	req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, string(gftperrors.ErrListFailed), resp.Errors[0].Code)
	assert.NotContains(t, resp.Errors[0].Message, "weird unexpected error", "raw error must not leak")
}

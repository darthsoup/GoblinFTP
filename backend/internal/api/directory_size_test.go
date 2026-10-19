package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func sizeMock(tree map[string][]transfer.FileInfo) *testutil.MockClient {
	return &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		LstatFn:      func(string) (transfer.FileInfo, error) { return transfer.FileInfo{IsDir: true}, nil },
		ListFn: func(p string) ([]transfer.FileInfo, error) {
			entries, ok := tree[p]
			if !ok {
				return nil, fmt.Errorf("%w: private remote message", transfer.ErrPermissionDenied)
			}
			return entries, nil
		},
	}
}

func sizeApp(t *testing.T, mock *testutil.MockClient) (*echo.Echo, *auth.Store, sessionCtx) {
	t.Helper()
	app, store, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	t.Cleanup(store.Close)
	return app, store, connectAndGetSession(t, app)
}

func readDirectorySize(t *testing.T, rec *httptest.ResponseRecorder) transfer.DirectorySizeResult {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var envelope struct {
		Success bool                         `json:"success"`
		Data    transfer.DirectorySizeResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)
	return envelope.Data
}

func requestDirectorySize(app http.Handler, sess sessionCtx, ctx context.Context, p string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"path": p})
	req := httptest.NewRequest(http.MethodPost, "/api/files/directory-size", strings.NewReader(string(body))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func awaitDirectoryScan(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("directory scan did not reach the expected operation")
	}
}

func awaitDirectoryResponse(t *testing.T, responses <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case rec := <-responses:
		return rec
	case <-time.After(3 * time.Second):
		t.Fatal("directory scan did not finish")
		return nil
	}
}

func sessionForSize(t *testing.T, store *auth.Store, ctx sessionCtx) *auth.Session {
	t.Helper()
	for _, cookie := range ctx.cookies {
		if cookie.Name == api.SessionCookieName {
			sess, ok := store.Get(cookie.Value)
			require.True(t, ok)
			return sess
		}
	}
	t.Fatal("session cookie missing")
	return nil
}

func TestDirectorySizeRecursiveAndEmpty(t *testing.T) {
	mock := sizeMock(map[string][]transfer.FileInfo{
		"/root": {
			{Name: "a", Size: 5}, {Name: ".hidden", Size: 3}, {Name: "child", IsDir: true},
			{Name: "link", Size: 100, IsSymlink: true}, {Name: "dir-link", IsDir: true, IsSymlink: true},
		},
		"/root/child": {{Name: "b", Size: 8}},
		"/empty":      {},
	})
	app, _, sess := sizeApp(t, mock)
	result := readDirectorySize(t, requestDirectorySize(app, sess, context.Background(), "/root"))
	assert.Equal(t, "/root", result.Path)
	assert.Equal(t, int64(16), result.Bytes)
	assert.Equal(t, 3, result.FileCount)
	assert.Equal(t, 1, result.DirectoryCount)
	assert.True(t, result.Complete)
	assert.Zero(t, result.SkippedCount)
	_, err := time.Parse(time.RFC3339, result.CalculatedAt)
	require.NoError(t, err)

	empty := readDirectorySize(t, requestDirectorySize(app, sess, context.Background(), "/empty"))
	assert.True(t, empty.Complete)
	assert.Zero(t, empty.Bytes)
	assert.Zero(t, empty.FileCount)
	assert.Zero(t, empty.DirectoryCount)
}

func TestDirectorySizePartialAndOverflow(t *testing.T) {
	mock := sizeMock(map[string][]transfer.FileInfo{"/root": {
		{Name: "denied", IsDir: true}, {Name: "large", Size: math.MaxInt64}, {Name: "overflow", Size: 1},
	}})
	app, _, sess := sizeApp(t, mock)
	result := readDirectorySize(t, requestDirectorySize(app, sess, context.Background(), "/root"))
	assert.Equal(t, int64(math.MaxInt64), result.Bytes)
	assert.Equal(t, 1, result.FileCount)
	assert.Equal(t, 1, result.DirectoryCount)
	assert.Equal(t, 2, result.SkippedCount)
	assert.Equal(t, "limit", result.Reason)
	assert.False(t, result.Complete)
}

func TestDirectorySizeRejectsInvalidInput(t *testing.T) {
	app, _, sess := sizeApp(t, sizeMock(nil))
	for _, body := range []string{`{`, `{}`, `{"path":"relative"}`, `{"path":"/a/../b"}`, `{"path":"/a\nLIST /"}`} {
		rec := doJSON(app, sess, http.MethodPost, "/api/files/directory-size", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "ERR_BAD_REQUEST")
	}
}

func TestDirectorySizeRootErrorsAreClassified(t *testing.T) {
	for _, tc := range []struct {
		name string
		info transfer.FileInfo
		err  error
		code string
	}{
		{name: "file", code: "ERR_INVALID_TYPE"},
		{name: "symlink", info: transfer.FileInfo{IsDir: true, IsSymlink: true}, code: "ERR_INVALID_TYPE"},
		{name: "missing", err: transfer.ErrNotFound, code: "ERR_FILE_NOT_FOUND"},
		{name: "denied", err: transfer.ErrPermissionDenied, code: "ERR_FILE_PERMISSION"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := sizeMock(nil)
			mock.LstatFn = func(string) (transfer.FileInfo, error) {
				if tc.err != nil {
					return tc.info, fmt.Errorf("%w: private remote text", tc.err)
				}
				return tc.info, nil
			}
			app, _, sess := sizeApp(t, mock)
			rec := requestDirectorySize(app, sess, context.Background(), "/root")
			assert.NotEqual(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.code)
			assert.NotContains(t, rec.Body.String(), "private remote text")
		})
	}
	app, _, sess := sizeApp(t, sizeMock(nil))
	rec := requestDirectorySize(app, sess, context.Background(), "/denied")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_FILE_PERMISSION")
	assert.NotContains(t, rec.Body.String(), "private remote message")
}

func TestDirectorySizeRequiresSessionAndCSRF(t *testing.T) {
	app, _, sess := sizeApp(t, sizeMock(nil))
	for _, invalid := range []sessionCtx{{}, {cookies: sess.cookies}, {cookies: sess.cookies, csrfToken: "invalid"}} {
		rec := requestDirectorySize(app, invalid, context.Background(), "/root")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	}
}

func TestDirectorySizeConcurrentScansArePerSession(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	first := sizeMock(nil)
	first.ListContextFn = func(ctx context.Context, p string) ([]transfer.FileInfo, error) {
		close(started)
		select {
		case <-release:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	second := sizeMock(map[string][]transfer.FileInfo{"/root": {{Name: "a", Size: 2}}})
	var dialCount atomic.Int32
	app, store, _ := newTestApp(t, defaultTestConfig(), api.WithDial(func(api.DialRequest) (transfer.Client, *api.HostKeyPrompt, error) {
		if dialCount.Add(1) == 1 {
			return first, nil, nil
		}
		return second, nil, nil
	}))
	t.Cleanup(store.Close)
	firstSession, secondSession := connectAndGetSession(t, app), connectAndGetSession(t, app)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	responses := make(chan *httptest.ResponseRecorder, 1)
	go func() { responses <- requestDirectorySize(app, firstSession, ctx, "/root") }()
	awaitDirectoryScan(t, started)

	rec := requestDirectorySize(app, firstSession, context.Background(), "/other")
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_OPERATION_IN_PROGRESS")
	result := readDirectorySize(t, requestDirectorySize(app, secondSession, context.Background(), "/root"))
	assert.Equal(t, int64(2), result.Bytes)
	assert.True(t, result.Complete)
	cancel()
	awaitDirectoryResponse(t, responses)
}

func TestDirectorySizeCancellationKeepsSessionAndReleasesScan(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	mock := sizeMock(map[string][]transfer.FileInfo{"/root": {}})
	mock.ListContextFn = func(ctx context.Context, p string) ([]transfer.FileInfo, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return mock.List(p)
	}
	app, _, sess := sizeApp(t, mock)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	responses := make(chan *httptest.ResponseRecorder, 1)
	go func() { responses <- requestDirectorySize(app, sess, ctx, "/root") }()
	awaitDirectoryScan(t, started)
	cancel()
	awaitDirectoryResponse(t, responses)
	assert.False(t, mock.IsClosed())
	assert.True(t, getStatus(t, app, sess, "").Data.Connected)
	result := readDirectorySize(t, requestDirectorySize(app, sess, context.Background(), "/root"))
	assert.True(t, result.Complete)
}

func TestDirectorySizeCancellationWhileWaitingForTransfer(t *testing.T) {
	var remoteCalls atomic.Int32
	mock := sizeMock(map[string][]transfer.FileInfo{"/root": {}})
	mock.LstatFn = func(string) (transfer.FileInfo, error) {
		remoteCalls.Add(1)
		return transfer.FileInfo{IsDir: true}, nil
	}
	app, store, sess := sizeApp(t, mock)
	session := sessionForSize(t, store, sess)
	session.LockTransfer()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	requestDirectorySize(app, sess, ctx, "/root")
	session.UnlockTransfer()
	assert.Zero(t, remoteCalls.Load())
	assert.False(t, mock.IsClosed())
	assert.True(t, getStatus(t, app, sess, "").Data.Connected)
	result := readDirectorySize(t, requestDirectorySize(app, sess, context.Background(), "/root"))
	assert.True(t, result.Complete)
}

func TestDirectorySizeCancellationWaitsForCurrentLegacyList(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	mock := sizeMock(nil)
	mock.ListFn = func(string) ([]transfer.FileInfo, error) {
		calls.Add(1)
		close(started)
		<-release
		return []transfer.FileInfo{{Name: "child", IsDir: true}}, nil
	}
	app, _, sess := sizeApp(t, mock)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	responses := make(chan *httptest.ResponseRecorder, 1)
	go func() { responses <- requestDirectorySize(app, sess, ctx, "/root") }()
	awaitDirectoryScan(t, started)
	cancel()
	assert.False(t, mock.IsClosed())
	close(release)
	awaitDirectoryResponse(t, responses)
	assert.Equal(t, int32(1), calls.Load())
	assert.True(t, getStatus(t, app, sess, "").Data.Connected)
}

func TestDirectorySizeSessionRemovedBetweenListings(t *testing.T) {
	mock := sizeMock(map[string][]transfer.FileInfo{"/root": {{Name: "child", IsDir: true}}})
	app, store, sess := sizeApp(t, mock)
	session := sessionForSize(t, store, sess)
	mock.ListContextFn = func(ctx context.Context, p string) ([]transfer.FileInfo, error) {
		store.Delete(session.ID)
		return mock.List(p)
	}
	rec := requestDirectorySize(app, sess, context.Background(), "/root")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_SESSION_NOT_FOUND")
}

func TestDirectorySizeClientRemovedBetweenListings(t *testing.T) {
	mock := sizeMock(map[string][]transfer.FileInfo{"/root": {{Name: "child", IsDir: true}}})
	app, store, sess := sizeApp(t, mock)
	session := sessionForSize(t, store, sess)
	mock.ListContextFn = func(ctx context.Context, p string) ([]transfer.FileInfo, error) {
		session.Delete("client")
		return mock.List(p)
	}
	rec := requestDirectorySize(app, sess, context.Background(), "/root")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_SESSION_NOT_FOUND")
}

func TestDirectorySizeConnectionLossClosesClient(t *testing.T) {
	mock := sizeMock(map[string][]transfer.FileInfo{"/root": {{Name: "child", IsDir: true}}})
	mock.ListContextFn = func(ctx context.Context, p string) ([]transfer.FileInfo, error) {
		if p == "/root/child" {
			return nil, fmt.Errorf("%w: private server text", transfer.ErrConnectionLost)
		}
		return mock.List(p)
	}
	app, _, sess := sizeApp(t, mock)
	rec := requestDirectorySize(app, sess, context.Background(), "/root")
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_CONNECTION_LOST")
	assert.NotContains(t, rec.Body.String(), "private server text")
	assert.True(t, mock.IsClosed())
	assert.False(t, getStatus(t, app, sess, "").Data.Connected)
}

func TestDirectorySizeCancelledBrokenConnectionIsCleared(t *testing.T) {
	started := make(chan struct{})
	mock := sizeMock(nil)
	mock.ReadDirContextFn = func(ctx context.Context, _ string, _ int) ([]transfer.FileInfo, bool, error) {
		close(started)
		<-ctx.Done()
		return nil, false, errors.Join(transfer.ErrConnectionLost, ctx.Err())
	}
	app, store, sess := sizeApp(t, mock)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	responses := make(chan *httptest.ResponseRecorder, 1)
	go func() { responses <- requestDirectorySize(app, sess, ctx, "/root") }()
	awaitDirectoryScan(t, started)
	cancel()
	awaitDirectoryResponse(t, responses)
	assert.True(t, mock.IsClosed())
	assert.False(t, getStatus(t, app, sess, "").Data.Connected)
	session := sessionForSize(t, store, sess)
	require.True(t, session.BeginDirectoryScan())
	session.EndDirectoryScan()
}

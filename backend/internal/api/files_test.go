// backend/internal/api/files_test.go
package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

type sessionCtx struct {
	cookies   []*http.Cookie
	csrfToken string
}

func connectAndGetSession(t *testing.T, app *echo.Echo) sessionCtx {
	t.Helper()
	body := `{"protocol":"ftp","host":"h","port":21,"username":"u","password":"p"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "connect failed: %s", rec.Body.String())

	var resp struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return sessionCtx{cookies: rec.Result().Cookies(), csrfToken: resp.Data.CSRFToken}
}

func addSession(req *http.Request, sess sessionCtx) {
	for _, c := range sess.cookies {
		req.AddCookie(c)
	}
	if sess.csrfToken != "" {
		req.Header.Set(auth.CSRFHeaderName, sess.csrfToken)
	}
}

func TestListFiles(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		ListFn: func(path string) ([]transfer.FileInfo, error) {
			return []transfer.FileInfo{
				{Name: "file.txt", Size: 100, IsDir: false},
				{Name: "subdir", IsDir: true},
			}, nil
		},
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    []struct {
			Name  string `json:"name"`
			IsDir bool   `json:"isDir"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Len(t, resp.Data, 2)
	assert.Equal(t, "file.txt", resp.Data[0].Name)
	assert.Equal(t, "subdir", resp.Data[1].Name)
}

func TestCreateDirectory(t *testing.T) {
	var made []string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn:       func(string) (transfer.FileInfo, error) { return transfer.FileInfo{}, errors.New("not found") },
		MakeDirFn:    func(p string) error { made = append(made, p); return nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/directory", `{"path":"/newdir"}`)
	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, []string{"/newdir"}, made)
}

// TestCreateDirectoryNested: a multi-segment path creates each missing ancestor.
func TestCreateDirectoryNested(t *testing.T) {
	var made []string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn:       func(string) (transfer.FileInfo, error) { return transfer.FileInfo{}, errors.New("not found") },
		MakeDirFn:    func(p string) error { made = append(made, p); return nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/directory", `{"path":"/a/b/c"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, []string{"/a", "/a/b", "/a/b/c"}, made)
}

// TestCreateDirectoryIdempotent: an already-existing directory is a no-op success
// (raw FTP MakeDir would otherwise error).
func TestCreateDirectoryIdempotent(t *testing.T) {
	var made []string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn:       func(string) (transfer.FileInfo, error) { return transfer.FileInfo{IsDir: true}, nil },
		MakeDirFn:    func(p string) error { made = append(made, p); return nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/directory", `{"path":"/exists"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Empty(t, made, "existing dir must not be re-created")
}

// TestDeleteFilesPartialFailure: a batch where some items fail returns HTTP 200
// success:true with per-item results — the failure carries a classified code +
// friendly message, never the raw protocol string.
func TestDeleteFilesPartialFailure(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		DeleteFn: func(path string) error {
			if path == "/bad" {
				return fmt.Errorf(`550 "Remove directory operation failed."`)
			}
			return nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodDelete, "/api/files", `{"paths":["/good.txt","/bad"]}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Deleted []string `json:"deleted"`
			Failed  []struct {
				Path    string `json:"path"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"failed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, []string{"/good.txt"}, resp.Data.Deleted)
	require.Len(t, resp.Data.Failed, 1)
	assert.Equal(t, "/bad", resp.Data.Failed[0].Path)
	assert.Equal(t, string(gftperrors.ErrDirNotEmpty), resp.Data.Failed[0].Code)
	assert.NotContains(t, resp.Data.Failed[0].Message, "550", "raw protocol string must not leak")
}

// TestDeleteFilesAllFailStillSucceeds: even when every item fails, the response
// is HTTP 200 success:true so the SPA keeps the per-item data (it would discard
// data on success:false).
func TestDeleteFilesAllFailStillSucceeds(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		DeleteFn:     func(string) error { return fmt.Errorf("550 Permission denied") },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodDelete, "/api/files", `{"paths":["/a","/b"]}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Deleted []string `json:"deleted"`
			Failed  []struct {
				Code string `json:"code"`
			} `json:"failed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Empty(t, resp.Data.Deleted)
	require.Len(t, resp.Data.Failed, 2)
	assert.Equal(t, string(gftperrors.ErrFilePermission), resp.Data.Failed[0].Code)
}

// backend/internal/api/files_test.go
package api_test

import (
	"encoding/json"
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
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
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
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		MakeDirFn:    func(path string) error { return nil },
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	body := `{"path":"/newdir"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/directory", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDeleteFilesPartialFailure(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		DeleteFn: func(path string) error {
			if path == "/bad.txt" {
				return fmt.Errorf("permission denied")
			}
			return nil
		},
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	body := `{"paths":["/good.txt","/bad.txt"]}`
	req := httptest.NewRequest(http.MethodDelete, "/api/files", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMultiStatus, rec.Code)
}

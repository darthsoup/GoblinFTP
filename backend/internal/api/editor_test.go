package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func editorTestConfig() *config.Config {
	cfg := defaultTestConfig()
	cfg.Settings.Editor = config.EditorSettings{
		AllowedExtensions: []string{"txt", "js", "json"},
		Disabled:          false,
		ViewOnly:          false,
	}
	return cfg
}

func editorDialOption(mock *testutil.MockClient) api.HandlerOption {
	if mock.WorkingDirFn == nil {
		mock.WorkingDirFn = func() (string, error) { return "/", nil }
	}
	if mock.ChmodFn == nil {
		mock.ChmodFn = func(string, uint32) error { return nil }
	}
	return api.WithDial(func(p, a, u, pw string, passive bool) (transfer.Client, error) {
		return mock, nil
	})
}

func TestReadFile(t *testing.T) {
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		DownloadFn: func(path string) (io.ReadCloser, error) {
			assert.Equal(t, "/remote/file.txt", path)
			return io.NopCloser(strings.NewReader("hello world")), nil
		},
	}))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodGet, "/api/files/read?path=/remote/file.txt", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"hello world"`)
}

func TestReadFileDisallowedExtension(t *testing.T) {
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{}))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodGet, "/api/files/read?path=/remote/malware.exe", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestReadFileTooLarge(t *testing.T) {
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		DownloadFn: func(path string) (io.ReadCloser, error) {
			huge := strings.Repeat("x", 1*1024*1024+1)
			return io.NopCloser(strings.NewReader(huge)), nil
		},
	}))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodGet, "/api/files/read?path=/remote/big.txt", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_FILE_TOO_LARGE")
}

func TestReadFileEditorDisabled(t *testing.T) {
	cfg := editorTestConfig()
	cfg.Settings.Editor.Disabled = true
	app, _, _ := newTestApp(t, cfg, editorDialOption(&testutil.MockClient{}))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodGet, "/api/files/read?path=/remote/file.txt", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestWriteFile(t *testing.T) {
	var uploadedPath string
	var uploadedContent string
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		UploadFn: func(path string, r io.Reader) error {
			uploadedPath = path
			b, err := io.ReadAll(r)
			require.NoError(t, err)
			uploadedContent = string(b)
			return nil
		},
	}))
	sess := connectAndGetSession(t, app)

	body := `{"path":"/remote/file.txt","content":"updated content"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/write", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/remote/file.txt", uploadedPath)
	assert.Equal(t, "updated content", uploadedContent)
}

func TestWriteFileTooLarge(t *testing.T) {
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{}))
	sess := connectAndGetSession(t, app)

	huge := strings.Repeat("x", 1*1024*1024+1)
	body := `{"path":"/remote/file.txt","content":"` + huge + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/write", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_FILE_TOO_LARGE")
}

func TestWriteFileViewOnly(t *testing.T) {
	cfg := editorTestConfig()
	cfg.Settings.Editor.ViewOnly = true
	app, _, _ := newTestApp(t, cfg, editorDialOption(&testutil.MockClient{}))
	sess := connectAndGetSession(t, app)

	body := `{"path":"/remote/file.txt","content":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/write", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestWriteFileDisallowedExtension(t *testing.T) {
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{}))
	sess := connectAndGetSession(t, app)

	body := `{"path":"/remote/virus.exe","content":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/write", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestSystemVarsExposeEditorConfig(t *testing.T) {
	cfg := editorTestConfig()
	cfg.Settings.Editor.ViewOnly = true
	app, _, _ := newTestApp(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/system/vars", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Editor struct {
				Disabled          bool     `json:"disabled"`
				ViewOnly          bool     `json:"viewOnly"`
				AllowedExtensions []string `json:"allowedExtensions"`
			} `json:"editor"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.False(t, resp.Data.Editor.Disabled)
	assert.True(t, resp.Data.Editor.ViewOnly)
	assert.Equal(t, []string{"txt", "js", "json"}, resp.Data.Editor.AllowedExtensions)
}

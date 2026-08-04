package api_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/config"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

// The version token editorDialOption's default StatFn produces. Write tests that
// are not about conflict detection send this so the precondition passes.
const defaultEditorVersion = "11:1000"

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
	if mock.StatFn == nil {
		// Read and write both stat for the version token; tests that are not
		// about conflict detection get a stable one.
		mock.StatFn = func(string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Size: 11, ModTime: 1000}, nil
		}
	}
	return api.WithDial(staticDial(mock))
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

	body := `{"path":"/remote/file.txt","content":"updated content","expectedVersion":"` + defaultEditorVersion + `"}`
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

// TestReadFileDownloadError: a protocol error from the server is classified into
// a stable code + friendly message — the raw "550 ..." string must not leak.
func TestReadFileDownloadError(t *testing.T) {
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		DownloadFn: func(string) (io.ReadCloser, error) { return nil, errors.New("550 Permission denied") },
	}))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodGet, "/api/files/read?path=/remote/file.txt", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Errors)
	assert.Equal(t, string(gftperrors.ErrFilePermission), resp.Errors[0].Code)
	assert.NotContains(t, resp.Errors[0].Message, "550", "raw protocol string must not leak")
}

// TestWriteFileUploadError: same classification guarantee on the write path.
func TestWriteFileUploadError(t *testing.T) {
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		UploadFn: func(string, io.Reader) error { return errors.New("550 Permission denied") },
	}))
	sess := connectAndGetSession(t, app)

	body := `{"path":"/remote/file.txt","content":"x","expectedVersion":"` + defaultEditorVersion + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/write", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	var resp api.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Errors)
	assert.Equal(t, string(gftperrors.ErrFilePermission), resp.Errors[0].Code)
	assert.NotContains(t, resp.Errors[0].Message, "550", "raw protocol string must not leak")
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

// ── Edit-conflict detection ──────────────────────────────────────────────────

// statSeq returns a StatFn yielding each info in turn, repeating the last one.
// It models the file changing on the server between the open and the save.
func statSeq(infos ...transfer.FileInfo) func(string) (transfer.FileInfo, error) {
	i := 0
	return func(string) (transfer.FileInfo, error) {
		fi := infos[min(i, len(infos)-1)]
		i++
		return fi, nil
	}
}

func writeReq(t *testing.T, app http.Handler, sess sessionCtx, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/files/write", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func TestReadFileReturnsVersion(t *testing.T) {
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		StatFn:     func(string) (transfer.FileInfo, error) { return transfer.FileInfo{Size: 42, ModTime: 1712345678}, nil },
		DownloadFn: func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("body")), nil },
	}))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodGet, "/api/files/read?path=/remote/file.txt", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			Version  *string `json:"version"`
			Size     int64   `json:"size"`
			Modified string  `json:"modified"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.Version)
	assert.Equal(t, "42:1712345678", *resp.Data.Version)
	assert.Equal(t, int64(42), resp.Data.Size)
	assert.Equal(t, "2024-04-05T19:34:38Z", resp.Data.Modified)
}

// A file the server cannot stat still opens; the client is told it has no
// conflict detection via a null version rather than being blocked.
func TestReadFileUnstattableReturnsNullVersion(t *testing.T) {
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		StatFn:     func(string) (transfer.FileInfo, error) { return transfer.FileInfo{}, errors.New("550 not found") },
		DownloadFn: func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("body")), nil },
	}))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodGet, "/api/files/read?path=/remote/file.txt", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"version":null`)
}

func TestWriteFileRejectsStaleVersion(t *testing.T) {
	uploaded := false
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		StatFn:   func(string) (transfer.FileInfo, error) { return transfer.FileInfo{Size: 99, ModTime: 2000}, nil },
		UploadFn: func(string, io.Reader) error { uploaded = true; return nil },
	}))
	sess := connectAndGetSession(t, app)

	rec := writeReq(t, app, sess, `{"path":"/remote/file.txt","content":"mine","expectedVersion":"11:1000"}`)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), string(gftperrors.ErrFileModified))
	assert.False(t, uploaded, "a rejected save must not reach the server")
}

func TestWriteFileOverwriteBypassesPrecondition(t *testing.T) {
	uploaded := false
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		StatFn:   func(string) (transfer.FileInfo, error) { return transfer.FileInfo{Size: 99, ModTime: 2000}, nil },
		UploadFn: func(string, io.Reader) error { uploaded = true; return nil },
	}))
	sess := connectAndGetSession(t, app)

	rec := writeReq(t, app, sess, `{"path":"/remote/file.txt","content":"mine","overwrite":true}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, uploaded)
	assert.Contains(t, rec.Body.String(), `"99:2000"`, "the response carries the new baseline")
}

// The version returned after a save must describe the file as it is now, or the
// client's very next save conflicts with the write it just made.
func TestWriteFileReturnsRefreshedVersion(t *testing.T) {
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		StatFn: statSeq(
			transfer.FileInfo{Size: 11, ModTime: 1000},
			transfer.FileInfo{Size: 20, ModTime: 3000},
		),
		UploadFn: func(string, io.Reader) error { return nil },
	}))
	sess := connectAndGetSession(t, app)

	rec := writeReq(t, app, sess, `{"path":"/remote/file.txt","content":"longer content","expectedVersion":"11:1000"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"20:3000"`)
}

func TestWriteFileDeletedSinceOpen(t *testing.T) {
	uploaded := false
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		StatFn:   func(string) (transfer.FileInfo, error) { return transfer.FileInfo{}, errors.New("550 no such file") },
		UploadFn: func(string, io.Reader) error { uploaded = true; return nil },
	}))
	sess := connectAndGetSession(t, app)

	rec := writeReq(t, app, sess, `{"path":"/remote/file.txt","content":"mine","expectedVersion":"11:1000"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), string(gftperrors.ErrFileNotFound))
	assert.False(t, uploaded)
}

// Fail closed: a client that forgets the token must not silently lose the
// protection. This also stops Mod-S on a tab whose read failed from writing an
// empty file over a healthy one.
func TestWriteFileRequiresPrecondition(t *testing.T) {
	uploaded := false
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(&testutil.MockClient{
		UploadFn: func(string, io.Reader) error { uploaded = true; return nil },
	}))
	sess := connectAndGetSession(t, app)

	rec := writeReq(t, app, sess, `{"path":"/remote/file.txt","content":"mine"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), string(gftperrors.ErrBadRequest))
	assert.False(t, uploaded)
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

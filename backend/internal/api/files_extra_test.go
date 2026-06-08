// backend/internal/api/files_extra_test.go
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

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func TestRenameFile(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		RenameFn:     func(src, dst string) error { return nil },
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	body := `{"from":"/a.txt","to":"/b.txt"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/files/rename", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRenameFile_MissingFields(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodPatch, "/api/files/rename", strings.NewReader(`{"from":""}`))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCopyFile(t *testing.T) {
	content := "hello world"
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn:       func(string) (transfer.FileInfo, error) { return transfer.FileInfo{Name: "a.txt"}, nil },
		DownloadFn:   func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(content)), nil },
		UploadFn:     func(string, io.Reader) error { return nil },
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	body := `{"from":"/a.txt","to":"/b.txt"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/files/copy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// Copying a directory recurses: MakeDir for the destination tree + Upload for
// each contained file.
func TestCopyFile_Directory(t *testing.T) {
	var mkdirs, uploaded []string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn: func(p string) (transfer.FileInfo, error) {
			switch p {
			case "/src":
				return transfer.FileInfo{Name: "src", IsDir: true}, nil
			case "/src/file.txt":
				return transfer.FileInfo{Name: "file.txt"}, nil
			default:
				// destination paths don't exist yet
				return transfer.FileInfo{}, errors.New("not found")
			}
		},
		ListFn: func(string) ([]transfer.FileInfo, error) {
			return []transfer.FileInfo{{Name: "file.txt"}}, nil
		},
		MakeDirFn:  func(p string) error { mkdirs = append(mkdirs, p); return nil },
		DownloadFn: func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("data")), nil },
		UploadFn:   func(p string, _ io.Reader) error { uploaded = append(uploaded, p); return nil },
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	body := `{"from":"/src","to":"/dst"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/files/copy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"/dst"}, mkdirs)
	assert.Equal(t, []string{"/dst/file.txt"}, uploaded)
}

// trackedReader reports whether it is still open, so a test can assert the copy
// closed the download before starting the upload.
type trackedReader struct {
	io.Reader
	onClose func()
}

func (r *trackedReader) Close() error { r.onClose(); return nil }

// Regression: FTP allows only one data transfer per control connection, so a copy
// must fully close the download (RETR) before opening the upload (STOR). This
// mock fails the upload if the download is still open — catching a reintroduction
// of the streaming Download→Upload that desyncs the FTP control channel.
func TestCopyFile_ClosesDownloadBeforeUpload(t *testing.T) {
	downloadOpen := false
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn:       func(string) (transfer.FileInfo, error) { return transfer.FileInfo{Name: "a.txt"}, nil },
		DownloadFn: func(string) (io.ReadCloser, error) {
			downloadOpen = true
			return &trackedReader{Reader: strings.NewReader("data"), onClose: func() { downloadOpen = false }}, nil
		},
		UploadFn: func(string, io.Reader) error {
			if downloadOpen {
				return errors.New("interleaved transfer: download still open during upload")
			}
			return nil
		},
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	body := `{"from":"/a.txt","to":"/b.txt"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/files/copy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCopyFile_MissingFields(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodPatch, "/api/files/copy", strings.NewReader(`{"from":""}`))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSetPermissions(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	mode := uint32(0o755)
	bodyBytes, _ := json.Marshal(map[string]any{"path": "/a.txt", "mode": mode})
	req := httptest.NewRequest(http.MethodPatch, "/api/files/permissions", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSetPermissions_MissingFields(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodPatch, "/api/files/permissions", strings.NewReader(`{"path":""}`))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSetPermissions_NotSupported(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return transfer.ErrPermissionsNotSupported },
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	mode := uint32(0o755)
	bodyBytes, _ := json.Marshal(map[string]any{"path": "/a.txt", "mode": mode})
	req := httptest.NewRequest(http.MethodPatch, "/api/files/permissions", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

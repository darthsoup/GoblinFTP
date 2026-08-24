package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

// failingCloser delivers its content in full, then reports the truncation from
// Close, which is exactly how an FTP RETR reports a short transfer.
type failingCloser struct {
	r        io.Reader
	closeErr error
}

func (f *failingCloser) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *failingCloser) Close() error               { return f.closeErr }

// The reader's Close carries the server's final status, and discarding it made a
// truncated read indistinguishable from a complete one.
func TestEditorReadRejectsTruncatedRead(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		StatFn: func(string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Name: "a.txt", Size: 100, ModTime: 1700000000}, nil
		},
		DownloadFn: func(string) (io.ReadCloser, error) {
			return &failingCloser{
				r:        strings.NewReader("short"),
				closeErr: errors.New("426 transfer aborted"),
			}, nil
		},
	}
	app, _, _ := newTestApp(t, editorTestConfig(), editorDialOption(mock))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodGet, "/api/files/read?path=/remote/a.txt", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusOK, rec.Code,
		"a truncated read must not return content plus a version token")
	assert.Contains(t, rec.Body.String(), "ERR_TRANSFER_INCOMPLETE")
	assert.NotContains(t, rec.Body.String(), "short",
		"the partial content must not be handed to the editor")
}

// copyFile staged the download and uploaded whatever arrived, so a short read
// was written to the destination and reported as a successful copy.
func TestCopyAbortsOnTruncatedRead(t *testing.T) {
	uploaded := false
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		StatFn: func(string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Name: "a.txt", Size: 100}, nil
		},
		DownloadFn: func(string) (io.ReadCloser, error) {
			return &failingCloser{
				r:        strings.NewReader("short"),
				closeErr: errors.New("426 transfer aborted"),
			}, nil
		},
		UploadFn: func(string, io.Reader) error { uploaded = true; return nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodPatch, "/api/files/copy",
		strings.NewReader(`{"from":"/a.txt","to":"/b.txt"}`))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusOK, rec.Code)
	assert.False(t, uploaded, "a truncated source must never be uploaded to the destination")
	assert.Contains(t, rec.Body.String(), "ERR_TRANSFER_INCOMPLETE")
}

// Without a Content-Length a short chunked body terminates cleanly and the
// browser saves it as a complete file.
func TestDownloadSetsContentLength(t *testing.T) {
	content := "hello file content"
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		StatFn: func(string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Name: "f.txt", Size: int64(len(content))}, nil
		},
		DownloadFn: func(string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(content)), nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	token := issueDownloadToken(t, app, sess, "/f.txt")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/files/download?token="+token, nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "18", rec.Header().Get("Content-Length"))
	assert.Equal(t, content, rec.Body.String())
}

// A directory has to be refused before any header is written, or the failure
// arrives as a corrupt download instead of an error.
func TestDownloadRejectsDirectoryBeforeCommitting(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		StatFn: func(string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Name: "d", IsDir: true}, nil
		},
		DownloadFn: func(string) (io.ReadCloser, error) {
			return nil, transfer.ErrInvalidType
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	token := issueDownloadToken(t, app, sess, "/d")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/files/download?token="+token, nil))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_INVALID_TYPE")
	assert.Empty(t, rec.Header().Get("Content-Disposition"),
		"nothing may be committed before the refusal")
}

// destinationTaken used to treat every Stat failure as "free", so an
// indeterminate probe silently defeated the overwrite guard.
func TestUploadOverwriteGuardFailsClosed(t *testing.T) {
	uploaded := false
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		StatFn: func(p string) (transfer.FileInfo, error) {
			if p == "/uploads" {
				return transfer.FileInfo{IsDir: true}, nil
			}
			// Not "absent", just unknown: a refused LIST, a timeout, a hiccup.
			return transfer.FileInfo{}, errors.New("451 local error in processing")
		},
		MakeDirFn: func(string) error { return nil },
		UploadFn:  func(string, io.Reader) error { uploaded = true; return nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("path", "/uploads/x.txt")
	part, _ := writer.CreateFormFile("file", "x.txt")
	_, _ = io.WriteString(part, "data")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/files/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusOK, rec.Code)
	assert.False(t, uploaded, "an indeterminate probe must not clear the overwrite guard")
}

// issueDownloadToken mints a download token through the real endpoint, so the
// test exercises the same path the SPA does.
func issueDownloadToken(t *testing.T, app *echo.Echo, sess sessionCtx, path string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/files/download-token",
		strings.NewReader(`{"path":"`+path+`"}`))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "token: %s", rec.Body.String())

	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Data.Token
}

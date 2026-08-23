package api_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func TestExtractZipArchive(t *testing.T) {
	var uploadedFiles []string

	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		UploadFn: func(path string, r io.Reader) error {
			uploadedFiles = append(uploadedFiles, path)
			return nil
		},
		MakeDirFn: func(path string) error { return nil },
		StatFn: func(string) (transfer.FileInfo, error) {
			// Destination does not exist yet, so ensureDirAll creates it.
			return transfer.FileInfo{}, errors.New("not found")
		},
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, _ := zw.Create("hello.txt")
	_, _ = io.WriteString(w, "hello")
	zw.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("destination", "/extracted/")
	part, _ := writer.CreateFormFile("archive", "test.zip")
	_, _ = io.Copy(part, &zipBuf)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/files/extract", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Len(t, uploadedFiles, 1)
	assert.Equal(t, "/extracted/hello.txt", uploadedFiles[0])
}

func TestCreateZipArchive(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn: func(path string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Name: "file.txt", IsDir: false, Size: 5}, nil
		},
		DownloadFn: func(path string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("hello")), nil
		},
		UploadFn: func(path string, r io.Reader) error {
			// Must consume the reader to unblock the pipe
			_, _ = io.Copy(io.Discard, r)
			return nil
		},
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	body := `{"paths":["/file.txt"],"destination":"/archive.zip"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/compress", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// Regression: an Upload that errors without draining the io.Pipe reader used to
// block the writer goroutine forever, holding the session's transfer lock.
func TestCreateZipUploadFailsWithoutReading(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		StatFn: func(path string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Name: "file.txt", IsDir: false, Size: 5}, nil
		},
		DownloadFn: func(path string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("hello")), nil
		},
		ListFn: func(path string) ([]transfer.FileInfo, error) {
			return []transfer.FileInfo{{Name: "file.txt", Size: 5}}, nil
		},
		// Rejects the destination outright, the way an FTP 550 does: no bytes
		// are read from the reader at all.
		UploadFn: func(path string, r io.Reader) error {
			return errors.New("550 permission denied")
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	compress := func() *httptest.ResponseRecorder {
		body := `{"paths":["/file.txt"],"destination":"/archive.zip"}`
		req := httptest.NewRequest(http.MethodPost, "/api/files/compress", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		addSession(req, sess)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		return rec
	}

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- compress() }()

	select {
	case rec := <-done:
		assert.GreaterOrEqual(t, rec.Code, 400, "a rejected upload must surface as an error")
	case <-time.After(10 * time.Second):
		t.Fatal("CreateZip deadlocked when Upload failed without draining the archive")
	}

	// The failure must not have wedged the session's transfer lock: a second
	// request has to still be served. This is the part users actually feel.
	second := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
		addSession(req, sess)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		second <- rec.Code
	}()
	select {
	case code := <-second:
		assert.Equal(t, http.StatusOK, code, "session must stay usable after a failed compress")
	case <-time.After(10 * time.Second):
		t.Fatal("session transfer lock was never released - later requests block forever")
	}
}

// buildTarGz builds a .tar.gz in memory from name -> content.
func buildTarGz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return buf.Bytes()
}

func postArchive(t *testing.T, app *echo.Echo, sess sessionCtx, filename string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.WriteField("destination", "/extracted"))
	part, err := w.CreateFormFile("archive", filename)
	require.NoError(t, err)
	_, err = part.Write(payload)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/files/extract", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func TestExtractTarGzArchive(t *testing.T) {
	var uploaded []string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		MakeDirFn:    func(string) error { return nil },
		StatFn: func(string) (transfer.FileInfo, error) {
			// Destination does not exist yet, so ensureDirAll creates it.
			return transfer.FileInfo{}, errors.New("not found")
		},
		UploadFn: func(p string, r io.Reader) error {
			_, _ = io.Copy(io.Discard, r)
			uploaded = append(uploaded, p)
			return nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := postArchive(t, app, sess, "bundle.tar.gz", buildTarGz(t, map[string]string{"hello.txt": "hi"}))
	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, []string{"/extracted/hello.txt"}, uploaded)
}

// Covers safeJoin, the security boundary of the whole extract feature.
func TestExtractTarRejectsPathTraversal(t *testing.T) {
	for _, name := range []string{"../escaped.txt", "../../etc/passwd", "a/../../escaped.txt"} {
		t.Run(name, func(t *testing.T) {
			var uploaded []string
			mock := &testutil.MockClient{
				WorkingDirFn: func() (string, error) { return "/", nil },
				MakeDirFn:    func(string) error { return nil },
				StatFn: func(string) (transfer.FileInfo, error) {
					// Destination does not exist yet, so ensureDirAll creates it.
					return transfer.FileInfo{}, errors.New("not found")
				},
				UploadFn: func(p string, r io.Reader) error {
					_, _ = io.Copy(io.Discard, r)
					uploaded = append(uploaded, p)
					return nil
				},
			}
			app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
			sess := connectAndGetSession(t, app)

			rec := postArchive(t, app, sess, "evil.tar.gz", buildTarGz(t, map[string]string{name: "pwned"}))
			assert.GreaterOrEqual(t, rec.Code, 400, "traversal entry must be rejected")
			assert.Empty(t, uploaded, "nothing may be written outside the destination")
		})
	}
}

// maxZipSize bounds only the compressed upload, so an unbounded tar branch lets
// a small archive expand to gigabytes on the user's remote account.
func TestExtractRejectsDecompressionBomb(t *testing.T) {
	// ~600 MB of zeroes, which gzip squeezes into a very small upload.
	bomb := buildTarGz(t, map[string]string{"big.bin": strings.Repeat("\x00", 600*1024*1024)})
	t.Logf("compressed archive is %d bytes", len(bomb))

	var written int64
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		MakeDirFn:    func(string) error { return nil },
		StatFn: func(string) (transfer.FileInfo, error) {
			// Destination does not exist yet, so ensureDirAll creates it.
			return transfer.FileInfo{}, errors.New("not found")
		},
		UploadFn: func(p string, r io.Reader) error {
			n, err := io.Copy(io.Discard, r)
			written += n
			return err
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := postArchive(t, app, sess, "bomb.tar.gz", bomb)
	assert.GreaterOrEqual(t, rec.Code, 400, "an over-budget extraction must fail")
	assert.LessOrEqual(t, written, int64(512*1024*1024),
		"extraction wrote %d bytes, past the budget", written)
}

// FTP's MakeDir is single-level, so an archive with implicit nested directories
// (no explicit directory entries) needs every missing parent created in turn.
func TestExtractCreatesNestedDirectories(t *testing.T) {
	var made []string
	var uploaded []string
	existing := map[string]bool{}

	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		StatFn: func(p string) (transfer.FileInfo, error) {
			if existing[p] {
				return transfer.FileInfo{Name: path.Base(p), IsDir: true}, nil
			}
			return transfer.FileInfo{}, errors.New("not found")
		},
		// Single-level, exactly like the real FTP adapter: creating "/a/b"
		// fails unless "/a" already exists.
		MakeDirFn: func(p string) error {
			parent := path.Dir(p)
			if parent != "/" && parent != "." && !existing[parent] {
				return errors.New("550 no such parent directory")
			}
			existing[p] = true
			made = append(made, p)
			return nil
		},
		UploadFn: func(p string, r io.Reader) error {
			if !existing[path.Dir(p)] {
				return errors.New("550 no such directory")
			}
			_, _ = io.Copy(io.Discard, r)
			uploaded = append(uploaded, p)
			return nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	archive := buildTarGz(t, map[string]string{"a/b/c/deep.txt": "hi"})
	rec := postArchive(t, app, sess, "nested.tar.gz", archive)

	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, []string{"/extracted/a/b/c/deep.txt"}, uploaded)
	assert.Contains(t, made, "/extracted/a", "every missing parent must be created")
	assert.Contains(t, made, "/extracted/a/b")
	assert.Contains(t, made, "/extracted/a/b/c")
}

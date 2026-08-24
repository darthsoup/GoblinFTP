package api_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func extractRequest(t *testing.T, filename string, payload []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	require.NoError(t, w.WriteField("destination", "/dest"))
	part, err := w.CreateFormFile("archive", filename)
	require.NoError(t, err)
	_, err = part.Write(payload)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return body, w.FormDataContentType()
}

func extractMock() *testutil.MockClient {
	return &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		StatFn:       func(string) (transfer.FileInfo, error) { return transfer.FileInfo{IsDir: true}, nil },
		MakeDirFn:    func(string) error { return nil },
		UploadFn:     func(string, io.Reader) error { return nil },
	}
}

func postExtract(t *testing.T, mock *testutil.MockClient, filename string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	body, contentType := extractRequest(t, filename, payload)
	req := httptest.NewRequest(http.MethodPost, "/api/files/extract", body)
	req.Header.Set("Content-Type", contentType)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func makeTarGz(t *testing.T, entries func(*tar.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	entries(tw)
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return buf.Bytes()
}

// A truncated upload surfaced as io.ErrUnexpectedEOF, which isConnLost matched,
// so a corrupt payload closed the live FTP/SFTP session and showed the reconnect
// dialog instead of "your archive is broken".
func TestTruncatedArchiveDoesNotKillTheSession(t *testing.T) {
	full := makeTarGz(t, func(tw *tar.Writer) {
		content := bytes.Repeat([]byte("a"), 4096)
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: "big.txt", Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write(content)
		require.NoError(t, err)
	})

	mock := extractMock()
	// Cut the payload in half so the gzip stream ends mid-member.
	rec := postExtract(t, mock, "archive.tar.gz", full[:len(full)/2])

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "ERR_ARCHIVE_FORMAT")
	assert.False(t, mock.IsClosed(),
		"a corrupt upload is the caller's problem and must not close the server connection")
}

// A zip-slip entry is a rejection, not an upstream failure, and its name must
// not be echoed back into the response.
func TestZipSlipIsRejected(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../../etc/evil.conf")
	require.NoError(t, err)
	_, err = w.Write([]byte("pwned"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	mock := extractMock()
	rec := postExtract(t, mock, "evil.zip", buf.Bytes())

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_ARCHIVE_FORMAT")
	assert.NotContains(t, rec.Body.String(), "etc/evil.conf",
		"the rejected entry name must not be reflected to the caller")
	assert.False(t, mock.IsClosed())
}

// Entries no remote filesystem can represent are skipped and reported, not
// silently dropped as if they had been extracted.
func TestUnsupportedTarEntriesAreReported(t *testing.T) {
	payload := makeTarGz(t, func(tw *tar.Writer) {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: "link.txt", Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd",
		}))
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: "real.txt", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write([]byte("hi"))
		require.NoError(t, err)
	})

	rec := postExtract(t, extractMock(), "mixed.tar.gz", payload)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	body := rec.Body.String()
	assert.Contains(t, body, `"written":1`)
	assert.Contains(t, body, "symlink")
	assert.Contains(t, body, "link.txt")
}

func TestValidArchiveExtractsAndReportsCount(t *testing.T) {
	payload := makeTarGz(t, func(tw *tar.Writer) {
		for _, name := range []string{"a.txt", "b.txt"} {
			require.NoError(t, tw.WriteHeader(&tar.Header{
				Name: name, Mode: 0o644, Size: 2, Typeflag: tar.TypeReg,
			}))
			_, err := tw.Write([]byte("hi"))
			require.NoError(t, err)
		}
	})

	rec := postExtract(t, extractMock(), "ok.tar.gz", payload)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"written":2`)
	assert.Contains(t, rec.Body.String(), `"skipped":[]`)
}

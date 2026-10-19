package api_test

import (
	"archive/zip"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
)

func archivePathPayload(t *testing.T, format, name string) []byte {
	t.Helper()
	if format == "tar.gz" {
		return buildTarGz(t, map[string]string{name: "content"})
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	entry, err := w.Create(name)
	require.NoError(t, err)
	_, err = io.WriteString(entry, "content")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestExtractRejectsAmbiguousEntryPaths(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		for _, name := range []string{`..\outside.txt`, `folder\..\outside.txt`, `C:\outside.txt`, `C:outside.txt`, `\\host\share\file`, "/outside.txt", "folder/../file.txt", "../file.txt"} {
			t.Run(format+"/"+name, func(t *testing.T) {
				mock := extractMock()
				mock.MakeDirFn = func(string) error { t.Fatal("invalid entry created a directory"); return nil }
				mock.UploadFn = func(string, io.Reader) error { t.Fatal("invalid entry wrote a file"); return nil }
				rec := postExtract(t, mock, "archive."+format, archivePathPayload(t, format, name))
				assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
				assert.Contains(t, rec.Body.String(), "ERR_ARCHIVE_FORMAT")
			})
		}
	}
}

func TestExtractAllowsRootDestination(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			var written []string
			mock := extractMock()
			mock.UploadFn = func(p string, r io.Reader) error {
				written = append(written, p)
				_, err := io.Copy(io.Discard, r)
				return err
			}
			app, store, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
			defer store.Close()
			sess := connectAndGetSession(t, app)
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			require.NoError(t, writer.WriteField("destination", "/"))
			part, err := writer.CreateFormFile("archive", "archive."+format)
			require.NoError(t, err)
			_, err = part.Write(archivePathPayload(t, format, "nested/file.txt"))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			req := httptest.NewRequest(http.MethodPost, "/api/files/extract", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			addSession(req, sess)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, []string{"/nested/file.txt"}, written)
		})
	}
}

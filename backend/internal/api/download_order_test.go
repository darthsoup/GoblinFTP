package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func TestDownloadMetadataPrecedesStream(t *testing.T) {
	for _, content := range []string{"", strings.Repeat("data", 1024)} {
		t.Run(stringSizeName(content), func(t *testing.T) {
			opened := false
			stats := 0
			mock := &testutil.MockClient{
				WorkingDirFn: func() (string, error) { return "/", nil },
				StatFn: func(string) (transfer.FileInfo, error) {
					require.False(t, opened, "metadata must precede RETR")
					stats++
					return transfer.FileInfo{Name: "file.txt", Size: int64(len(content))}, nil
				},
				DownloadFn: func(string) (io.ReadCloser, error) {
					require.Equal(t, 1, stats)
					opened = true
					return io.NopCloser(strings.NewReader(content)), nil
				},
			}
			app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
			session := connectAndGetSession(t, app)
			token := issueDownloadToken(t, app, session, "/file.txt")
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/files/download?token="+token, nil))
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, content, rec.Body.String())
			if content != "" {
				require.NotEmpty(t, rec.Header().Get("Content-Length"))
			}
		})
	}
}

func TestDownloadZeroMetadataSizeCanStreamContent(t *testing.T) {
	const content = "symlink target content"
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		StatFn: func(string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Name: "link", Size: 0}, nil
		},
		DownloadFn: func(string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(content)), nil
		},
	}
	app, store, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	defer store.Close()
	session := connectAndGetSession(t, app)
	token := issueDownloadToken(t, app, session, "/link")
	server := httptest.NewServer(app)
	defer server.Close()
	resp, err := server.Client().Get(server.URL + "/api/files/download?token=" + token)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, content, string(got))
}

func stringSizeName(content string) string {
	if content == "" {
		return "empty"
	}
	return "nonempty"
}

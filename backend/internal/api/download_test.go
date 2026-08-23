package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func TestIssueDownloadToken(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	body := `{"path":"/file.txt"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/download-token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.Data.Token)
}

func TestDownloadFile(t *testing.T) {
	content := "hello file content"
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		DownloadFn: func(path string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(content)), nil
		},
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	tokenBody := `{"path":"/file.txt"}`
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/files/download-token", strings.NewReader(tokenBody))
	tokenReq.Header.Set("Content-Type", "application/json")
	addSession(tokenReq, sess)
	tokenRec := httptest.NewRecorder()
	app.ServeHTTP(tokenRec, tokenReq)
	require.Equal(t, http.StatusOK, tokenRec.Code)

	var tokenResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(tokenRec.Body.Bytes(), &tokenResp))
	token := tokenResp.Data.Token

	// The download route is public, so no session cookie is sent.
	dlReq := httptest.NewRequest(http.MethodGet, "/api/files/download?token="+token, nil)
	dlRec := httptest.NewRecorder()
	app.ServeHTTP(dlRec, dlReq)

	assert.Equal(t, http.StatusOK, dlRec.Code)
	assert.Equal(t, content, dlRec.Body.String())
}

func TestDownloadZip(t *testing.T) {
	content := "zip-me"
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn: func(path string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Name: "file.txt", Size: int64(len(content)), IsDir: false}, nil
		},
		DownloadFn: func(path string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(content)), nil
		},
	}
	dialFn := staticDial(mock)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	body := `{"paths":["/file.txt"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/download-zip", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/zip", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "archive.zip")

	body2 := rec.Body.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(body2), int64(len(body2)))
	require.NoError(t, err)
	require.Len(t, zr.File, 1)
	assert.Equal(t, "file.txt", zr.File[0].Name)
	rc, err := zr.File[0].Open()
	require.NoError(t, err)
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	assert.Equal(t, content, string(got))
}

func TestDownloadZipMissingPaths(t *testing.T) {
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(&testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
	})))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodPost, "/api/files/download-zip", strings.NewReader(`{"paths":[]}`))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDownloadZipRejectsOversizedArchive(t *testing.T) {
	const oversizedFile = 512*1024*1024 + 1
	calledDownload := false
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn: func(path string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Name: "large.bin", Size: oversizedFile, IsDir: false}, nil
		},
		DownloadFn: func(path string) (io.ReadCloser, error) {
			calledDownload = true
			return io.NopCloser(strings.NewReader("unexpected")), nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodPost, "/api/files/download-zip", strings.NewReader(`{"paths":["/large.bin"]}`))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, calledDownload)
}

// The token is signed, not encrypted, and download URLs travel where the HttpOnly
// cookie does not, so a session ID inside one is a session takeover.
func TestDownloadTokenDoesNotCarrySessionID(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		DownloadFn: func(string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("data")), nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	req := httptest.NewRequest(http.MethodPost, "/api/files/download-token",
		strings.NewReader(`{"path":"/file.txt"}`))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.NotEmpty(t, envelope.Data.Token)

	decoded, err := base64.RawURLEncoding.DecodeString(envelope.Data.Token)
	require.NoError(t, err, "token must be plain base64 - that is the whole point")
	var sessionID string
	for _, ck := range sess.cookies {
		if ck.Name == "gftp_session" {
			sessionID = ck.Value
		}
	}
	require.NotEmpty(t, sessionID)
	assert.NotContains(t, string(decoded), sessionID,
		"the session ID must never appear in a download token")

	subject := strings.SplitN(string(decoded), ":", 2)[0]
	require.NotEmpty(t, subject)
	replay := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
	replay.AddCookie(&http.Cookie{Name: "gftp_session", Value: subject})
	replayRec := httptest.NewRecorder()
	app.ServeHTTP(replayRec, replay)
	assert.NotEqual(t, http.StatusOK, replayRec.Code,
		"the token subject must not authenticate as a session")
}

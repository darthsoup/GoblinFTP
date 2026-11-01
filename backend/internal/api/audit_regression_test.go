package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/sso"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestConcurrentConnectCannotExceedAccountBudget(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.MaxConnectingPerIP = 40
	cfg.MaxSessionsPerIP = 100
	started := make(chan struct{}, 40)
	unblock := make(chan struct{})
	var calls atomic.Int32
	app, _, _ := newTestApp(t, cfg, api.WithDial(func(api.DialRequest) (transfer.Client, *api.HostKeyPrompt, error) {
		calls.Add(1)
		started <- struct{}{}
		<-unblock
		return nil, nil, transfer.ErrAuthFailed
	}))
	var done sync.WaitGroup
	for range 40 {
		done.Go(func() { rec := httptest.NewRecorder(); app.ServeHTTP(rec, newConnectRequest(t, validPayload())) })
	}
	for range cfg.LoginMaxAttempts {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			close(unblock)
			done.Wait()
			t.Fatal("expected admitted dial did not start")
		}
	}
	close(unblock)
	done.Wait()
	assert.EqualValues(t, cfg.LoginMaxAttempts, calls.Load())
}

func TestZipPreflightRejectsEntryBombAndForgedCounts(t *testing.T) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for range 10001 {
		_, err := writer.Create("x")
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	mock := extractMock()
	mock.UploadFn = func(string, io.Reader) error { t.Fatal("oversized metadata reached remote upload"); return nil }
	rec := postExtract(t, mock, "bomb.zip", buf.Bytes())
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	payload := archivePathPayload(t, "zip", "normal.txt")
	eocd := bytes.LastIndex(payload, []byte{'P', 'K', 5, 6})
	require.GreaterOrEqual(t, eocd, 0)
	binary.LittleEndian.PutUint16(payload[eocd+8:], 0)
	binary.LittleEndian.PutUint16(payload[eocd+10:], 0)
	rec = postExtract(t, mock, "forged.zip", payload)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
}

func TestColonArchiveNameIsAllowed(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		rec := postExtract(t, extractMock(), "archive."+format, archivePathPayload(t, format, "colon:name.txt"))
		assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestWindowsDeletePathsAndTraversalRejectBatch(t *testing.T) {
	mock := workingMock()
	mock.DeleteFn = func(string) error { t.Fatal("invalid batch reached remote delete"); return nil }
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)
	for _, target := range []string{`\/file.txt`, `\folder`, `C:/file`, `/C:/file`, "/.. ", "/. ", "/...", "/sub/.. ", "/file.", "/file "} {
		body, _ := json.Marshal(map[string]any{"paths": []string{"/ordinary", target}})
		rec := doJSON(app, sess, http.MethodDelete, "/api/files", string(body))
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestBackendRejectsCopyAndMoveOverlap(t *testing.T) {
	mock := workingMock()
	mock.MakeDirFn = func(string) error { t.Fatal("overlap created directory"); return nil }
	mock.RenameFn = func(string, string) error { t.Fatal("overlap renamed source"); return nil }
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)
	for _, endpoint := range []string{"copy", "rename"} {
		for _, target := range []string{"/a", "/a/child", "/A. /child"} {
			body, _ := json.Marshal(map[string]string{"from": "/a", "to": target})
			rec := doJSON(app, sess, http.MethodPatch, "/api/files/"+endpoint, string(body))
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		}
	}
}

func TestSSOLinkDoesNotReplaceActiveSessionBeforeConfirmation(t *testing.T) {
	cfg := ssoEnabledConfig()
	mock := workingMock()
	app, store, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
	active := connectAndGetSession(t, app)
	req := httptest.NewRequest(http.MethodGet, "/?sso="+validSSO(t, cfg.SSOSecret), nil)
	addSession(req, active)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)
	for _, cookie := range rec.Result().Cookies() {
		assert.Equal(t, "gftp_sso", cookie.Name)
	}
	status := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	addSession(status, active)
	statusRec := httptest.NewRecorder()
	app.ServeHTTP(statusRec, status)
	assert.Contains(t, statusRec.Body.String(), `"connected":true`)
	assert.Equal(t, 2, store.Count())
}

func TestDuplicateCookiesAreRejectedAndHTTPSIgnoresLegacy(t *testing.T) {
	app, _, _ := newTestApp(t, defaultTestConfig(), staticDialOption())
	sess := connectAndGetSession(t, app)
	req := httptest.NewRequest(http.MethodGet, "/api/files?path=/", nil)
	addSession(req, sess)
	req.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: sess.cookies[0].Value})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestTemporaryStorageQuotaRejectsBeforeMultipartParsing(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.DataDir = t.TempDir()
	cfg.StagingMaxBytes = 16
	cfg.StagingMaxSessionBytes = 16
	app, _, _ := newTestApp(t, cfg, staticDialOption())
	sess := connectAndGetSession(t, app)
	req := httptest.NewRequest(http.MethodPost, "/api/files/upload", strings.NewReader(strings.Repeat("x", 17)))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
}

func TestSSOAppliesDirectoryLanguageAndRotatesSession(t *testing.T) {
	cfg := ssoEnabledConfig()
	mock := workingMock()
	mock.StatFn = func(p string) (transfer.FileInfo, error) { return transfer.FileInfo{IsDir: p == "/start"}, nil }
	app, store, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
	payload := &sso.Payload{Type: "ftp", Host: "h", Port: 21, Username: "u", Password: "p", InitialDirectory: "/start", Language: "de", Exp: time.Now().Add(time.Minute).Unix()}
	token, err := sso.Encrypt(payload, cfg.SSOSecret)
	require.NoError(t, err)
	pending, csrf := establishSSO(t, app, token)
	rec := doSSOConnect(app, pending, csrf, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var response struct {
		Data api.ConnectData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "/start", response.Data.InitialDirectory)
	assert.Equal(t, "de", response.Data.Language)
	assert.NotEmpty(t, response.Data.ConnectionID)
	assert.NotEqual(t, csrf, response.Data.CSRFToken)
	_, exists := store.Get(pending.Value)
	assert.False(t, exists)
	var active *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == api.SessionCookieName {
			active = cookie
		}
	}
	require.NotNil(t, active)
	assert.NotEqual(t, pending.Value, active.Value)
}

func TestSSOCancelPreservesExistingConnection(t *testing.T) {
	cfg := ssoEnabledConfig()
	app, store, _ := newTestApp(t, cfg, staticDialOption())
	active := connectAndGetSession(t, app)
	pending, csrf := establishSSO(t, app, validSSO(t, cfg.SSOSecret))
	req := httptest.NewRequest(http.MethodDelete, "/api/auth/sso-pending", nil)
	addSession(req, active)
	req.AddCookie(pending)
	req.Header.Set(auth.CSRFHeaderName, csrf)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	_, exists := store.Get(pending.Value)
	assert.False(t, exists)
	assert.Equal(t, 1, store.Count())
	status := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	addSession(status, active)
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, status)
	assert.Contains(t, rec.Body.String(), `"connected":true`)
}

func TestHiddenPresetUsesConfiguredTarget(t *testing.T) {
	cfg := defaultTestConfig()
	host := "internal.example.com"
	port := 2121
	cfg.Settings.Connection.PresetHost = &host
	cfg.Settings.Connection.PresetPort = &port
	cfg.Settings.Connection.LockHost = true
	var dialed api.DialRequest
	app, _, _ := newTestApp(t, cfg, api.WithDial(func(req api.DialRequest) (transfer.Client, *api.HostKeyPrompt, error) {
		dialed = req
		return workingMock(), nil, nil
	}))
	vars := httptest.NewRecorder()
	app.ServeHTTP(vars, httptest.NewRequest(http.MethodGet, "/api/system/vars", nil))
	assert.NotContains(t, vars.Body.String(), host)
	body := validPayload()
	body.Host = ""
	body.Port = 0
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, newConnectRequest(t, body))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, host, dialed.Host)
	assert.Equal(t, "internal.example.com:2121", dialed.Addr)
}

func TestZIP64PreflightAcceptsBoundedStandardDirectory(t *testing.T) {
	payload := archivePathPayload(t, "zip", "file.txt")
	end := bytes.LastIndex(payload, []byte{'P', 'K', 5, 6})
	require.GreaterOrEqual(t, end, 0)
	directorySize := binary.LittleEndian.Uint32(payload[end+12:])
	directoryOffset := binary.LittleEndian.Uint32(payload[end+16:])
	record := make([]byte, 56)
	binary.LittleEndian.PutUint32(record, 0x06064b50)
	binary.LittleEndian.PutUint64(record[4:], 44)
	binary.LittleEndian.PutUint16(record[12:], 45)
	binary.LittleEndian.PutUint16(record[14:], 45)
	binary.LittleEndian.PutUint64(record[24:], 1)
	binary.LittleEndian.PutUint64(record[32:], 1)
	binary.LittleEndian.PutUint64(record[40:], uint64(directorySize))
	binary.LittleEndian.PutUint64(record[48:], uint64(directoryOffset))
	locator := make([]byte, 20)
	binary.LittleEndian.PutUint32(locator, 0x07064b50)
	binary.LittleEndian.PutUint64(locator[8:], uint64(end))
	binary.LittleEndian.PutUint32(locator[16:], 1)
	trailer := append([]byte(nil), payload[end:]...)
	binary.LittleEndian.PutUint16(trailer[8:], 0xffff)
	binary.LittleEndian.PutUint16(trailer[10:], 0xffff)
	binary.LittleEndian.PutUint32(trailer[12:], 0xffffffff)
	binary.LittleEndian.PutUint32(trailer[16:], 0xffffffff)
	zip64 := append(append(append(append([]byte(nil), payload[:end]...), record...), locator...), trailer...)
	rec := postExtract(t, extractMock(), "archive.zip", zip64)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestReservedUploadRejectsSizeMismatchAndParentEscape(t *testing.T) {
	mock := workingMock()
	mock.UploadFn = func(string, io.Reader) error { t.Fatal("invalid upload reached remote write"); return nil }
	chunks := newMemChunkStore()
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)), api.WithChunkStore(chunks))
	sess := connectAndGetSession(t, app)
	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/reserve", `{"path":"/dir/file","totalSize":11,"totalChunks":2,"chunkSize":5}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	rec = doJSON(app, sess, http.MethodPost, "/api/files/upload/reserve", `{"path":"/dir/file","totalSize":10,"totalChunks":2,"chunkSize":5,"overwrite":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var reserved struct {
		Data struct {
			ID string `json:"uploadId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &reserved))
	body, _ := json.Marshal(map[string]any{"uploadId": reserved.Data.ID, "destination": `/dir/other\..\escape`})
	rec = doJSON(app, sess, http.MethodPost, "/api/files/upload/commit", string(body))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	body, _ = json.Marshal(map[string]any{"uploadId": reserved.Data.ID})
	rec = doJSON(app, sess, http.MethodPost, "/api/files/upload/commit", string(body))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestCopySpoolingCannotBypassQuotaThroughReaderFrom(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.DataDir = t.TempDir()
	cfg.StagingMaxBytes = 4
	cfg.StagingMaxSessionBytes = 4
	mock := workingMock()
	mock.StatFn = func(string) (transfer.FileInfo, error) { return transfer.FileInfo{Name: "source", Size: 1}, nil }
	mock.DownloadFn = func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("ten bytes!")), nil }
	mock.UploadFn = func(string, io.Reader) error { t.Fatal("over-quota copy reached destination"); return nil }
	app, _, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)
	rec := doJSON(app, sess, http.MethodPatch, "/api/files/copy", `{"from":"/source","to":"/destination"}`)
	require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
}

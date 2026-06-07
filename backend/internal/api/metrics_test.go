// backend/internal/api/metrics_test.go
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

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/metrics"
	"github.com/darthsoup/goblinftp/internal/transfer"
	mocks "github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func workingMock() *mocks.MockClient {
	return &mocks.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
	}
}

func mockDial(client transfer.Client, err error) api.HandlerOption {
	return api.WithDial(func(string, string, string, string, bool) (transfer.Client, error) {
		return client, err
	})
}

func TestMetricsHTTPRequestCounter(t *testing.T) {
	m := metrics.New()
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithMetrics(m))
	defer store.Close()

	// Unauthenticated request on a real route → labeled with its template + 401.
	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	e.ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, float64(1), testutil.ToFloat64(m.HTTPRequests.WithLabelValues("GET", "/api/files", "401")))

	// Duration histogram observed for the same route.
	count, err := testutil.GatherAndCount(m.Registry, "gftp_http_request_duration_seconds")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 1)

	// Unrouted request → bounded label, never the raw URL.
	rawURL := "/totally/bogus/user-supplied-junk-12345"
	req = httptest.NewRequest(http.MethodGet, rawURL, nil)
	e.ServeHTTP(httptest.NewRecorder(), req)
	families, err := m.Registry.Gather()
	require.NoError(t, err)
	for _, mf := range families {
		if mf.GetName() != "gftp_http_requests_total" {
			continue
		}
		for _, metric := range mf.GetMetric() {
			for _, label := range metric.GetLabel() {
				assert.NotEqual(t, rawURL, label.GetValue(), "raw URLs must never become label values")
			}
		}
	}

	// /healthz is excluded entirely.
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	e.ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, float64(0), testutil.ToFloat64(m.HTTPRequests.WithLabelValues("GET", "/healthz", "200")))
}

func TestMetricsConnectSuccess(t *testing.T) {
	m := metrics.New()
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithMetrics(m), mockDial(workingMock(), nil))
	defer store.Close()

	connectAndGetSession(t, e)
	assert.Equal(t, float64(1), testutil.ToFloat64(m.ConnectAttempts.WithLabelValues("ftp", "success")))
}

func TestMetricsConnectAuthFailed(t *testing.T) {
	m := metrics.New()
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithMetrics(m), mockDial(nil, transfer.ErrAuthFailed))
	defer store.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/connect",
		strings.NewReader(`{"protocol":"ftp","host":"h","port":21,"username":"u","password":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, float64(1), testutil.ToFloat64(m.ConnectAttempts.WithLabelValues("ftp", "auth_failed")))
}

func TestMetricsConnectFailedAndThrottled(t *testing.T) {
	m := metrics.New()
	cfg := defaultTestConfig() // LoginMaxAttempts: 5
	e, store, _ := newTestApp(t, cfg, api.WithMetrics(m), mockDial(nil, errors.New("dial: connection refused")))
	defer store.Close()

	// 5 failed dials reach the throttle limit; the 6th is rejected up front.
	for range 6 {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/connect",
			strings.NewReader(`{"protocol":"ftp","host":"h","port":21,"username":"u","password":"p"}`))
		req.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(httptest.NewRecorder(), req)
	}

	assert.Equal(t, float64(5), testutil.ToFloat64(m.ConnectAttempts.WithLabelValues("ftp", "failed")))
	assert.Equal(t, float64(1), testutil.ToFloat64(m.ConnectAttempts.WithLabelValues("ftp", "throttled")))
}

func TestMetricsDownloadBytes(t *testing.T) {
	payload := strings.Repeat("d", 2048)
	mock := workingMock()
	mock.DownloadFn = func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(payload)), nil
	}
	m := metrics.New()
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithMetrics(m), mockDial(mock, nil))
	defer store.Close()
	sess := connectAndGetSession(t, e)

	// Issue a token, then download through the public endpoint.
	req := httptest.NewRequest(http.MethodPost, "/api/files/download-token", strings.NewReader(`{"path":"/file.bin"}`))
	req.Header.Set("Content-Type", "application/json")
	addSession(req, sess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var tokResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tokResp))

	req = httptest.NewRequest(http.MethodGet, "/api/files/download?token="+tokResp.Data.Token, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, payload, rec.Body.String())

	assert.Equal(t, float64(len(payload)), testutil.ToFloat64(m.TransferBytes.WithLabelValues("download", "ftp")))
}

func TestMetricsUploadBytes(t *testing.T) {
	payload := strings.Repeat("u", 4096)
	mock := workingMock()
	mock.UploadFn = func(_ string, r io.Reader) error {
		_, err := io.Copy(io.Discard, r)
		return err
	}
	m := metrics.New()
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithMetrics(m), mockDial(mock, nil))
	defer store.Close()
	sess := connectAndGetSession(t, e)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("path", "/up.bin")
	part, _ := writer.CreateFormFile("file", "up.bin")
	_, _ = io.WriteString(part, payload)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/files/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addSession(req, sess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	assert.Equal(t, float64(len(payload)), testutil.ToFloat64(m.TransferBytes.WithLabelValues("upload", "ftp")))
}

func TestMetricsFrontendErrorsCounter(t *testing.T) {
	m := metrics.New()
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithMetrics(m))
	defer store.Close()

	accepted := httptest.NewRequest(http.MethodPost, "/api/log/frontend",
		strings.NewReader(`{"kind":"error","message":"boom"}`))
	accepted.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(httptest.NewRecorder(), accepted)
	assert.Equal(t, float64(1), testutil.ToFloat64(m.FrontendErrors))

	rejected := httptest.NewRequest(http.MethodPost, "/api/log/frontend",
		strings.NewReader(`{"kind":"sneaky","message":"boom"}`))
	rejected.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(httptest.NewRecorder(), rejected)
	assert.Equal(t, float64(1), testutil.ToFloat64(m.FrontendErrors), "rejected kinds must not count")
}

func TestMetricsSessionGauges(t *testing.T) {
	m := metrics.New()
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithMetrics(m), mockDial(workingMock(), nil))
	defer store.Close()

	sess := connectAndGetSession(t, e)
	assert.NoError(t, testutil.GatherAndCompare(m.Registry, strings.NewReader(`
# HELP gftp_sessions_active Live authenticated sessions, computed from the session store at scrape time.
# TYPE gftp_sessions_active gauge
gftp_sessions_active 1
# HELP gftp_connections_active Live FTP/SFTP connections by protocol, computed at scrape time.
# TYPE gftp_connections_active gauge
gftp_connections_active{protocol="ftp"} 1
gftp_connections_active{protocol="sftp"} 0
`), "gftp_sessions_active", "gftp_connections_active"))

	req := httptest.NewRequest(http.MethodPost, "/api/auth/disconnect", nil)
	addSession(req, sess)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	assert.NoError(t, testutil.GatherAndCompare(m.Registry, strings.NewReader(`
# HELP gftp_sessions_active Live authenticated sessions, computed from the session store at scrape time.
# TYPE gftp_sessions_active gauge
gftp_sessions_active 0
# HELP gftp_connections_active Live FTP/SFTP connections by protocol, computed at scrape time.
# TYPE gftp_connections_active gauge
gftp_connections_active{protocol="ftp"} 0
gftp_connections_active{protocol="sftp"} 0
`), "gftp_sessions_active", "gftp_connections_active"))
}

// TestMetricsEndpointSmoke serves the registry exactly like main.go's
// dedicated listener does and checks the classic series are exposed.
func TestMetricsEndpointSmoke(t *testing.T) {
	m := metrics.New()
	e, store, _ := newTestApp(t, defaultTestConfig(), api.WithMetrics(m))
	defer store.Close()

	// Generate one request so the HTTP series exist.
	e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/system/vars", nil))

	srv := httptest.NewServer(promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	for _, series := range []string{"gftp_http_requests_total", "gftp_sessions_active", "gftp_connections_active", "go_goroutines"} {
		assert.Contains(t, string(body), series)
	}
}

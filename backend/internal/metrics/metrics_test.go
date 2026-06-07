package metrics

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountingReaderCountsExactly(t *testing.T) {
	c := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_bytes"})
	src := strings.NewReader(strings.Repeat("x", 10_000))

	// Tiny buffer forces many partial reads.
	n, err := io.CopyBuffer(io.Discard, CountingReader(src, c), make([]byte, 7))
	require.NoError(t, err)
	assert.Equal(t, int64(10_000), n)
	assert.Equal(t, float64(10_000), testutil.ToFloat64(c))
}

type failingReader struct {
	data io.Reader
	left int
}

func (f *failingReader) Read(p []byte) (int, error) {
	if f.left <= 0 {
		return 0, errors.New("stream died")
	}
	if len(p) > f.left {
		p = p[:f.left]
	}
	n, err := f.data.Read(p)
	f.left -= n
	return n, err
}

func TestCountingReaderCountsBytesBeforeMidStreamFailure(t *testing.T) {
	c := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_bytes"})
	src := &failingReader{data: strings.NewReader(strings.Repeat("x", 1000)), left: 300}

	_, err := io.Copy(io.Discard, CountingReader(src, c))
	require.Error(t, err)
	assert.Equal(t, float64(300), testutil.ToFloat64(c), "bytes read before the failure must be counted")
}

func TestCountingReaderNilCounterPassthrough(t *testing.T) {
	src := strings.NewReader("abc")
	assert.Equal(t, io.Reader(src), CountingReader(src, nil))
}

func TestCollectorZeroBeforeSnapshotWired(t *testing.T) {
	m := New()
	series, err := testutil.GatherAndCount(m.Registry, "gftp_sessions_active", "gftp_connections_active")
	require.NoError(t, err)
	assert.Equal(t, 3, series, "1 sessions + 2 protocol series present even before wiring")

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

func TestCollectorReportsSnapshot(t *testing.T) {
	m := New()
	m.SetConnectionSnapshot(func() Snapshot {
		return Snapshot{Sessions: 3, ConnsByProtocol: map[string]int{"ftp": 2, "sftp": 1}}
	})

	assert.NoError(t, testutil.GatherAndCompare(m.Registry, strings.NewReader(`
# HELP gftp_sessions_active Live authenticated sessions, computed from the session store at scrape time.
# TYPE gftp_sessions_active gauge
gftp_sessions_active 3
# HELP gftp_connections_active Live FTP/SFTP connections by protocol, computed at scrape time.
# TYPE gftp_connections_active gauge
gftp_connections_active{protocol="ftp"} 2
gftp_connections_active{protocol="sftp"} 1
`), "gftp_sessions_active", "gftp_connections_active"))
}

func TestRegistryNamingLint(t *testing.T) {
	m := New()
	// Touch the vectors so they emit at least one series each.
	m.HTTPRequests.WithLabelValues("GET", "/api/files", "200").Inc()
	m.HTTPDuration.WithLabelValues("GET", "/api/files").Observe(0.01)
	m.ConnectAttempts.WithLabelValues("ftp", "success").Inc()
	m.TransferBytes.WithLabelValues("download", "ftp").Add(42)
	m.FrontendErrors.Inc()

	problems, err := testutil.GatherAndLint(m.Registry)
	require.NoError(t, err)
	assert.Empty(t, problems, "prometheus naming lint must pass: %v", problems)
}

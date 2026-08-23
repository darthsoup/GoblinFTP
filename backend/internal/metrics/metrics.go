// Package metrics owns the Prometheus registry and all GoblinFTP series. /metrics is
// served by a dedicated listener in cmd/gftp, never on the main echo server.
package metrics

import (
	"io"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const namespace = "gftp"

// Snapshot is a point-in-time view of the session store, taken at scrape time
// by the connection collector (no inc/dec drift).
type Snapshot struct {
	Sessions        int
	ConnsByProtocol map[string]int // keys: "ftp", "sftp"
}

// Metrics bundles the registry and every instrumented series.
type Metrics struct {
	Registry        *prometheus.Registry
	HTTPRequests    *prometheus.CounterVec
	HTTPDuration    *prometheus.HistogramVec
	ConnectAttempts *prometheus.CounterVec
	TransferBytes   *prometheus.CounterVec
	FrontendErrors  prometheus.Counter

	conns *connCollector
}

// New builds a Metrics instance with its own private registry (never the global
// default, which keeps tests isolated). Gauges read zero until SetConnectionSnapshot.
func New() *Metrics {
	m := &Metrics{
		Registry: prometheus.NewRegistry(),
		HTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "http", Name: "requests_total",
			Help: "HTTP requests by method, route template, and status code.",
		}, []string{"method", "path", "status"}),
		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "http", Name: "request_duration_seconds",
			Help:    "HTTP request duration in seconds by method and route template.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),
		ConnectAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "connect_attempts_total",
			Help: "FTP/SFTP dial attempts by protocol and result (success, auth_failed, failed, throttled).",
		}, []string{"protocol", "result"}),
		TransferBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "transfer_bytes_total",
			Help: "File bytes transferred between browser and the connected server, by direction and protocol.",
		}, []string{"direction", "protocol"}),
		FrontendErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "frontend_errors_total",
			Help: "Browser-side error reports accepted on /api/log/frontend.",
		}),
		conns: newConnCollector(),
	}
	m.Registry.MustRegister(
		m.HTTPRequests, m.HTTPDuration, m.ConnectAttempts, m.TransferBytes, m.FrontendErrors,
		m.conns,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// SetConnectionSnapshot wires the scrape-time source for the session and
// connection gauges (called by the API handler, which owns the session store).
func (m *Metrics) SetConnectionSnapshot(fn func() Snapshot) {
	m.conns.set(fn)
}

// CountingReader wraps r so bytes count as they are read: a transfer that fails
// mid-stream still counts what actually moved. A nil counter returns r unchanged.
func CountingReader(r io.Reader, counter prometheus.Counter) io.Reader {
	if counter == nil {
		return r
	}
	return &countingReader{r: r, c: counter}
}

type countingReader struct {
	r io.Reader
	c prometheus.Counter
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	if n > 0 {
		cr.c.Add(float64(n))
	}
	return n, err
}

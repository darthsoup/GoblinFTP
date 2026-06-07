package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// connCollector emits the session/connection gauges from ONE store snapshot
// per scrape, so both series are consistent from the same instant. Both
// protocol series are always emitted (0 when absent) so they never vanish
// from the scrape between connections.
type connCollector struct {
	mu sync.RWMutex
	fn func() Snapshot

	sessions *prometheus.Desc
	conns    *prometheus.Desc
}

func newConnCollector() *connCollector {
	return &connCollector{
		sessions: prometheus.NewDesc(
			namespace+"_sessions_active",
			"Live authenticated sessions, computed from the session store at scrape time.",
			nil, nil,
		),
		conns: prometheus.NewDesc(
			namespace+"_connections_active",
			"Live FTP/SFTP connections by protocol, computed at scrape time.",
			[]string{"protocol"}, nil,
		),
	}
}

func (c *connCollector) set(fn func() Snapshot) {
	c.mu.Lock()
	c.fn = fn
	c.mu.Unlock()
}

func (c *connCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.sessions
	ch <- c.conns
}

func (c *connCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	fn := c.fn
	c.mu.RUnlock()

	var snap Snapshot
	if fn != nil {
		snap = fn()
	}
	ch <- prometheus.MustNewConstMetric(c.sessions, prometheus.GaugeValue, float64(snap.Sessions))
	for _, proto := range []string{"ftp", "sftp"} {
		ch <- prometheus.MustNewConstMetric(c.conns, prometheus.GaugeValue, float64(snap.ConnsByProtocol[proto]), proto)
	}
}

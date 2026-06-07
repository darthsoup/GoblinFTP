// backend/internal/api/middleware_metrics.go
package api

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/metrics"
)

// metricsMiddleware records one counter increment + one duration observation
// per request. It must sit OUTSIDE requestLogger (registered before it): the
// logger's c.Error(err) call commits echo-level errors, so by the time this
// middleware's post-next code runs the response status is always final. It
// must NOT call c.Error itself — the logger owns error handling.
func metricsMiddleware(m *metrics.Metrics) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().URL.Path == "/healthz" {
				return next(c) // polled by the container entrypoint; excluded
			}
			start := time.Now()
			err := next(c)

			// c.Path() is the route template — bounded cardinality. Unrouted
			// requests yield router-node prefixes (still finite) or "";
			// only the empty case needs a sentinel. Real routed 404s
			// (e.g. ERR_FILE_NOT_FOUND) keep their true template.
			path := c.Path()
			if path == "" {
				path = "unmatched"
			}
			method := c.Request().Method
			m.HTTPRequests.WithLabelValues(method, path, strconv.Itoa(c.Response().Status)).Inc()
			m.HTTPDuration.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
			return err
		}
	}
}

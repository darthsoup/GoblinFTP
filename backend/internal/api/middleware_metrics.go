package api

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/metrics"
)

// metricsMiddleware records one counter increment and one duration observation
// per request. It must sit OUTSIDE requestLogger (final status) and never c.Error.
func metricsMiddleware(m *metrics.Metrics) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().URL.Path == "/healthz" {
				return next(c) // polled by the container entrypoint; excluded
			}
			start := time.Now()
			err := next(c)

			// c.Path() is the route template, so cardinality stays bounded. Unrouted
			// requests yield a finite router prefix or "", the only case needing a sentinel.
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

// backend/cmd/gftp/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/logging"
	"github.com/darthsoup/goblinftp/internal/metrics"
	gftpsentry "github.com/darthsoup/goblinftp/internal/sentry"
	"github.com/darthsoup/goblinftp/internal/staging"
)

func newApp(cfg *config.Config, opts ...api.HandlerOption) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true // the port is logged structured in main; keep stdout pure JSON
	e.Use(gftpsentry.Middleware())

	store := auth.NewStore(time.Duration(cfg.SessionTTLSeconds) * time.Second)
	throttle := auth.NewThrottle()
	api.Register(e, cfg, store, throttle, opts...)

	return e
}

// newS3Store builds the optional S3 chunk-staging backend and probes the
// bucket. An unreachable bucket logs a warning but does not block startup.
func newS3Store(cfg *config.Config, logger *slog.Logger) *staging.S3Store {
	s3store := staging.NewS3Store(staging.S3Options{
		Endpoint:     cfg.S3Endpoint,
		Bucket:       cfg.S3Bucket,
		Region:       cfg.S3Region,
		AccessKey:    cfg.S3AccessKey,
		SecretKey:    cfg.S3SecretKey,
		UsePathStyle: cfg.S3UsePathStyle,
		Prefix:       cfg.S3Prefix,
		Timeout:      time.Duration(cfg.S3TimeoutSeconds) * time.Second,
	})
	if err := s3store.Ping(context.Background()); err != nil {
		logger.Warn("S3 chunk staging enabled but bucket is not reachable — uploads will fail until it is",
			"endpoint", cfg.S3Endpoint, "bucket", cfg.S3Bucket, "error", err.Error())
	} else {
		logger.Info("S3 chunk staging enabled", "endpoint", cfg.S3Endpoint, "bucket", cfg.S3Bucket)
	}
	return s3store
}

func main() {
	// Bootstrap logger at default level to capture config-load warnings
	// (stdout-only, so this Init cannot fail).
	logger, _, _ := logging.Init(logging.Options{Level: "info"})

	settingsPath := os.Getenv("GFTP_SETTINGS_PATH")
	if settingsPath == "" {
		settingsPath = "/app/data/settings.json"
	}

	cfg, err := config.Load(logger, settingsPath)
	if err != nil {
		logger.Error("failed to load configuration", "error", err.Error())
		os.Exit(1)
	}

	// Re-init logger with the configured level/format and optional file sink.
	full, closeLog, logErr := logging.Init(logging.Options{
		Level:          cfg.LogLevel,
		Format:         cfg.LogFormat,
		File:           cfg.LogFile,
		FileMaxSizeMB:  cfg.LogFileMaxSizeMB,
		FileMaxBackups: cfg.LogFileMaxBackups,
		FileMaxAgeDays: cfg.LogFileMaxAgeDays,
	})
	if logErr != nil {
		logger.Error("failed to initialise logging", "error", logErr.Error())
		os.Exit(1)
	}
	logger = full
	defer func() { _ = closeLog() }()
	slog.SetDefault(logger)
	logger.Info("starting GoblinFTP",
		"port", cfg.Port, "log_level", cfg.LogLevel, "log_format", cfg.LogFormat, "log_file", cfg.LogFile)

	sentryRate, _ := strconv.ParseFloat(os.Getenv("GFTP_SENTRY_SAMPLE_RATE"), 64)
	if initErr := gftpsentry.Init(
		cfg.SentryDSN,
		os.Getenv("GFTP_SENTRY_ENVIRONMENT"),
		os.Getenv("GFTP_SENTRY_RELEASE"),
		sentryRate,
	); initErr != nil {
		logger.Warn("sentry init failed", "error", initErr.Error())
	}
	defer gftpsentry.Flush()

	opts := []api.HandlerOption{api.WithLogger(logger)}
	if cfg.S3Enabled {
		opts = append(opts, api.WithChunkStore(newS3Store(cfg, logger)))
	}

	// Optional Prometheus metrics on a dedicated listener — never on the main
	// server (Caddy does not proxy it). newApp wires the session store into
	// the shared instance via SetConnectionSnapshot.
	var m *metrics.Metrics
	if cfg.MetricsEnabled {
		m = metrics.New()
		opts = append(opts, api.WithMetrics(m))
	}

	e := newApp(cfg, opts...)

	if cfg.MetricsEnabled {
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
			logger.Info("metrics listening", "port", cfg.MetricsPort)
			srv := &http.Server{Addr: ":" + cfg.MetricsPort, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
			logger.Error("metrics server stopped", "error", srv.ListenAndServe().Error())
		}()
	}

	logger.Error("server stopped", "error", e.Start(":"+cfg.Port).Error())
}

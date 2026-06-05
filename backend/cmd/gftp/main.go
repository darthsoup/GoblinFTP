// backend/cmd/gftp/main.go
package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/logging"
	gftpsentry "github.com/darthsoup/goblinftp/internal/sentry"
	"github.com/darthsoup/goblinftp/internal/staging"
)

func newApp(cfg *config.Config, opts ...api.HandlerOption) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
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
	// Bootstrap logger at default level to capture config-load warnings.
	logger := logging.Init("info")

	settingsPath := os.Getenv("GFTP_SETTINGS_PATH")
	if settingsPath == "" {
		settingsPath = "/app/data/settings.json"
	}

	cfg, err := config.Load(logger, settingsPath)
	if err != nil {
		logger.Error("failed to load configuration", "error", err.Error())
		os.Exit(1)
	}

	// Re-init logger with configured level.
	logger = logging.Init(cfg.LogLevel)
	logger.Info("starting GoblinFTP", "port", cfg.Port, "log_level", cfg.LogLevel)

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

	var opts []api.HandlerOption
	if cfg.S3Enabled {
		opts = append(opts, api.WithChunkStore(newS3Store(cfg, logger)))
	}

	e := newApp(cfg, opts...)
	logger.Error("server stopped", "error", e.Start(":"+cfg.Port).Error())
}

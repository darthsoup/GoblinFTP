// backend/cmd/gftp/main.go
package main

import (
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/logging"
	gftpsentry "github.com/darthsoup/goblinftp/internal/sentry"
)

func newApp(cfg *config.Config) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(gftpsentry.Middleware())

	store := auth.NewStore(time.Duration(cfg.SessionTTLSeconds) * time.Second)
	throttle := auth.NewThrottle()
	api.Register(e, cfg, store, throttle)

	return e
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

	e := newApp(cfg)
	logger.Error("server stopped", "error", e.Start(":"+cfg.Port).Error())
}

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
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

// version is the build version, injected by release builds via
// `-ldflags "-X main.version=<tag>"` (see docker/Dockerfile).
var version = "dev"

// shutdownGrace bounds the drain: a transfer wedged on an unresponsive remote must
// not outlive the orchestrator's kill timeout (Docker SIGKILLs 10s after SIGTERM).
const shutdownGrace = 20 * time.Second

func newApp(cfg *config.Config, opts ...api.HandlerOption) (*echo.Echo, *auth.Store, *auth.Throttle, *api.Handler) {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true // the port is logged structured in main; keep stdout pure JSON
	// No WriteTimeout on purpose: an absolute deadline on the whole response would
	// abort long downloads and archive streams. Stalled writers: see download.go.
	e.Server.ReadHeaderTimeout = 10 * time.Second
	e.Server.IdleTimeout = 120 * time.Second
	// Echo's default RealIP() trusts the leftmost X-Forwarded-For entry from anyone.
	// A proxy allowlist walks right to left, so throttles see the real client.
	e.IPExtractor = api.IPExtractor(cfg)

	store := auth.NewStore(time.Duration(cfg.SessionTTLSeconds) * time.Second)
	throttle := auth.NewThrottle()
	h := api.Register(e, cfg, store, throttle, opts...)

	return e, store, throttle, h
}

// liveUploadIDs reports whether an upload is still referenced by a session, so the
// sweeper never reclaims committable chunks. Lock order: store then session.
func liveUploadIDs(store *auth.Store) func(string) bool {
	return func(uploadID string) bool {
		found := false
		store.Range(func(sess *auth.Session) {
			if found {
				return
			}
			for _, id := range sess.UploadIDs() {
				if id == uploadID {
					found = true
					return
				}
			}
		})
		return found
	}
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
		logger.Warn("S3 chunk staging enabled but bucket is not reachable - uploads will fail until it is",
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

	cfg, err := config.Load(logger)
	if err != nil {
		logger.Error("failed to load configuration", "error", err.Error())
		os.Exit(1)
	}

	full, closeLog, logErr := logging.Init(logging.Options{
		Level:          cfg.LogLevel,
		Format:         cfg.LogFormat,
		File:           cfg.LogFile,
		FileMaxSizeMB:  cfg.LogFileMaxSizeMB,
		FileMaxBackups: cfg.LogFileMaxBackups,
		FileMaxAgeDays: cfg.LogFileMaxAgeDays,
	})
	if logErr != nil {
		logger.Error("failed to initialize logging", "error", logErr.Error())
		os.Exit(1)
	}
	logger = full
	defer func() { _ = closeLog() }()
	slog.SetDefault(logger)
	logger.Info("starting GoblinFTP",
		"version", version,
		"port", cfg.Port, "log_level", cfg.LogLevel, "log_format", cfg.LogFormat, "log_file", cfg.LogFile)

	// A cross-site embed fails in several ways that look identical from the browser
	// (redirect loop, no console error), so state the policy to check Set-Cookie against.
	if cfg.EmbeddingEnabled() {
		logger.Info("iframe embedding enabled",
			"frame_ancestors", strings.Join(cfg.FrameAncestors, " "),
			"session_cookie", "SameSite=None; Secure; Partitioned",
			"chromeless", cfg.Settings.Embed.Chromeless,
			"note", "requires HTTPS; Safari blocks third-party cookies, so a same-registrable-domain embed is recommended")
	}

	// Explicit GFTP_SENTRY_RELEASE wins; release builds default to the tag.
	sentryRelease := cfg.SentryRelease
	if sentryRelease == "" {
		sentryRelease = version
	}
	if initErr := gftpsentry.Init(gftpsentry.Options{
		DSN:                cfg.SentryDSN,
		Environment:        cfg.SentryEnvironment,
		Release:            sentryRelease,
		TracesSampleRate:   cfg.SentrySampleRate,
		ErrorSampleRate:    cfg.SentryErrorSampleRate,
		SendSessionContext: cfg.SentrySendSessionContext,
	}); initErr != nil {
		logger.Warn("sentry init failed", "error", initErr.Error())
	}
	defer gftpsentry.Flush()

	opts := []api.HandlerOption{api.WithLogger(logger), api.WithVersion(version)}
	if cfg.S3Enabled {
		opts = append(opts, api.WithChunkStore(newS3Store(cfg, logger)))
	}

	// Optional Prometheus metrics on a dedicated listener, never on the main server
	// (Caddy does not proxy it). newApp wires the store in via SetConnectionSnapshot.
	var m *metrics.Metrics
	if cfg.MetricsEnabled {
		m = metrics.New()
		opts = append(opts, api.WithMetrics(m))
	}

	e, store, throttle, handler := newApp(cfg, opts...)

	// Local staging only. A restart loses the in-memory session store, so uploads
	// reserved before it are orphaned on disk with nothing left to reference them.
	var sweeper *staging.Sweeper
	if !cfg.S3Enabled {
		sweeper = staging.NewSweeper(cfg.DataDir, logger, liveUploadIDs(store))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var metricsSrv *http.Server
	if cfg.MetricsEnabled {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
		metricsSrv = &http.Server{Addr: ":" + cfg.MetricsPort, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			defer gftpsentry.Recover()
			logger.Info("metrics listening", "port", cfg.MetricsPort)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("metrics server stopped", "error", err.Error())
			}
		}()
	}

	go func() {
		defer gftpsentry.Recover()
		if err := e.Start(":" + cfg.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err.Error())
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down", "grace_seconds", int(shutdownGrace.Seconds()))

	// Ordering matters: stop accepting and let in-flight requests drain first,
	// so EvictAll is not closing connections under running transfers.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Warn("graceful shutdown timed out, closing anyway", "error", err.Error())
	}
	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(shutdownCtx)
	}

	// Closes each session's FTP/SFTP connection properly instead of letting
	// process death sever it, leaving the remote server to time it out.
	store.EvictAll()
	store.Close()
	throttle.Close()
	handler.Close()
	if sweeper != nil {
		sweeper.Close()
	}
	logger.Info("stopped")
}

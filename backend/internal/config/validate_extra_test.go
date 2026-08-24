package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/config"
)

// The metrics listener binds before the main server, so an identical port stole
// the main bind and the process exited without ever serving the app.
func TestMetricsPortMustDifferFromMainPort(t *testing.T) {
	t.Setenv("GFTP_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GFTP_DOWNLOAD_TOKEN_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GFTP_DATA_DIR", t.TempDir())
	t.Setenv("GFTP_METRICS_ENABLED", "true")
	t.Setenv("GFTP_PORT", "8080")
	t.Setenv("GFTP_METRICS_PORT", "8080")

	_, err := config.Load(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "GFTP_METRICS_PORT")
}

func TestMetricsPortDifferentIsAccepted(t *testing.T) {
	t.Setenv("GFTP_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GFTP_DOWNLOAD_TOKEN_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GFTP_DATA_DIR", t.TempDir())
	t.Setenv("GFTP_METRICS_ENABLED", "true")
	t.Setenv("GFTP_PORT", "8080")
	t.Setenv("GFTP_METRICS_PORT", "9090")

	cfg, err := config.Load(nil)

	require.NoError(t, err)
	assert.Equal(t, "9090", cfg.MetricsPort)
}

// Metrics off means the port is never bound, so a clash is irrelevant.
func TestMetricsPortClashIgnoredWhenMetricsDisabled(t *testing.T) {
	t.Setenv("GFTP_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GFTP_DOWNLOAD_TOKEN_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GFTP_DATA_DIR", t.TempDir())
	t.Setenv("GFTP_METRICS_ENABLED", "false")
	t.Setenv("GFTP_PORT", "8080")
	t.Setenv("GFTP_METRICS_PORT", "8080")

	_, err := config.Load(nil)
	assert.NoError(t, err)
}

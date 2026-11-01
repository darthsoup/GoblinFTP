package config

import (
	"fmt"
	"net"
)

func auditKeys() []Key {
	return group("Resource protection", "configuration",
		integer("GFTP_MAX_SESSIONS", func(c *Config) *int { return &c.MaxSessions }, positive(), doc("Maximum sessions, including pending logins.")),
		integer("GFTP_MAX_SESSIONS_PER_IP", func(c *Config) *int { return &c.MaxSessionsPerIP }, positive(), doc("Maximum sessions per client IP.")),
		integer("GFTP_MAX_CONNECTING", func(c *Config) *int { return &c.MaxConnecting }, positive(), doc("Maximum simultaneous remote connection attempts.")),
		integer("GFTP_MAX_CONNECTING_PER_IP", func(c *Config) *int { return &c.MaxConnectingPerIP }, positive(), doc("Maximum simultaneous connection attempts per client IP.")),
		int64Key("GFTP_STAGING_MAX_BYTES", func(c *Config) *int64 { return &c.StagingMaxBytes }, positive(), doc("Maximum temporary storage bytes across uploads and spooled operations.")),
		int64Key("GFTP_STAGING_MAX_SESSION_BYTES", func(c *Config) *int64 { return &c.StagingMaxSessionBytes }, positive(), doc("Maximum temporary storage bytes per session.")),
		integer("GFTP_UPLOAD_MAX_PENDING", func(c *Config) *int { return &c.UploadMaxPending }, positive(), doc("Maximum pending uploads per session.")),
		boolean("GFTP_CONNECTION_EXPOSE_PRESET", func(c *Config) *bool { return &c.ExposePreset }, doc("Expose configured server addresses in the anonymous login form.")),
		custom("GFTP_METRICS_BIND_ADDRESS", func(c *Config, raw string) error {
			if net.ParseIP(raw) == nil {
				return fmt.Errorf("GFTP_METRICS_BIND_ADDRESS must be an IP address")
			}
			c.MetricsBindAddress = raw
			return nil
		}, doc("Metrics listener IP address. Defaults to loopback.")),
	)
}

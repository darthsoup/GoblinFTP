package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Stale names from old releases become actionable startup errors instead of
// silently ignored values. Delete at 1.0.
var (
	renamedEnv = map[string]string{
		"GFTP_PAGE_TITLE":                   "GFTP_UI_PAGE_TITLE",
		"GFTP_APP_NAME":                     "GFTP_BRANDING_APP_NAME",
		"GFTP_LOGO_URL":                     "GFTP_BRANDING_LOGO_URL",
		"GFTP_LOGO_DARK_URL":                "GFTP_BRANDING_LOGO_DARK_URL",
		"GFTP_FAVICON_URL":                  "GFTP_BRANDING_FAVICON_URL",
		"GFTP_PRIMARY_COLOR":                "GFTP_BRANDING_PRIMARY_COLOR",
		"GFTP_PRIMARY_TEXT_COLOR":           "GFTP_BRANDING_PRIMARY_TEXT_COLOR",
		"GFTP_HIDE_ATTRIBUTION":             "GFTP_BRANDING_HIDE_ATTRIBUTION",
		"GFTP_FTP_TLS_INSECURE_SKIP_VERIFY": "GFTP_CONNECTION_FTP_TLS_INSECURE_SKIP_VERIFY",
		"GFTP_CHUNK_SIZE":                   "GFTP_UPLOAD_CHUNK_SIZE",
		"GFTP_MAX_CONCURRENT_UPLOADS":       "GFTP_UPLOAD_MAX_CONCURRENT",
		"GFTP_SESSION_TTL_SECS":             "GFTP_SESSION_TTL_SECONDS",
		"GFTP_LOGIN_COOLDOWN_SECS":          "GFTP_LOGIN_COOLDOWN_SECONDS",
		"GFTP_DISABLE_LOGIN_FORM":           "GFTP_LOGIN_FORM_DISABLED",
		"GFTP_S3_TIMEOUT_SECS":              "GFTP_S3_TIMEOUT_SECONDS",
	}
	removedEnv = map[string]string{
		"GFTP_LOGIN_DISABLED_REDIRECT": "was removed (it was documented but never functional)",
		"GFTP_SETTINGS_PATH":           "was removed together with settings.json support — configure via environment variables (see docs/migration-0.24.md)",
	}
)

// RenamedEnv exposes the old-to-new env name map for the generated migration
// guide, so the documented table provably matches the runtime detection.
func RenamedEnv() map[string]string { return renamedEnv }

// RemovedEnv exposes the removed env names with their migration hints, for the
// generated migration guide.
func RemovedEnv() map[string]string { return removedEnv }

func defaultConfig() *Config {
	return &Config{
		Port:               "8080",
		LogLevel:           "info",
		LogFormat:          "json",
		LogFileMaxSizeMB:   10,
		LogFileMaxBackups:  5,
		FrontendLogEnabled: true,
		MetricsPort:        "9091",
		ChunkSize:          5 * 1024 * 1024,
		// Default 1: one control connection serves one transfer at a time, so
		// higher values mostly queue on the per-session transfer lock.
		MaxConcurrentUploads: 1,
		// Container data volume; `just dev-be` injects GFTP_DATA_DIR=data so
		// local dev writes to <repo>/data instead.
		DataDir:              "/app/data",
		LoginMaxAttempts:     5,
		LoginCooldownSeconds: 300,
		SessionTTLSeconds:    7200,
		S3Region:             "us-east-1",
		S3UsePathStyle:       true,
		S3Prefix:             "gftp-uploads",
		S3TimeoutSeconds:     60,
		Settings:             defaultSettings(),
	}
}

// Load resolves configuration in three passes: defaults, env vars, cross-key
// validation. An empty env var counts as unset.
func Load(logger *slog.Logger) (*Config, error) {
	if err := checkStaleEnv(); err != nil {
		return nil, err
	}

	cfg := defaultConfig()

	for i := range Registry {
		k := &Registry[i]
		if k.fromEnv == nil {
			continue
		}
		raw := os.Getenv(k.Env)
		if raw == "" {
			continue
		}
		if err := k.fromEnv(cfg, raw); err != nil {
			return nil, err
		}
	}

	if err := checkStaleSettingsFile(cfg.DataDir); err != nil {
		return nil, err
	}

	for i := range Registry {
		if fn := Registry[i].applyAutogen; fn != nil {
			if err := fn(cfg, logger); err != nil {
				return nil, err
			}
		}
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func checkStaleEnv() error {
	for old, newName := range renamedEnv {
		if os.Getenv(old) != "" {
			return fmt.Errorf("%s was renamed to %s — update your environment (see docs/migration-0.24.md)", old, newName)
		}
	}
	for old, hint := range removedEnv {
		if os.Getenv(old) != "" {
			return fmt.Errorf("%s %s", old, hint)
		}
	}
	return nil
}

// checkStaleSettingsFile fails startup when a settings.json is still mounted;
// silently ignoring it is exactly the failure class this config system kills.
func checkStaleSettingsFile(dataDir string) error {
	path := filepath.Join(dataDir, "settings.json")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s exists but settings.json is no longer read — move its values to environment variables and remove the file (see docs/migration-0.24.md)", path)
	}
	return nil
}

// validate holds the cross-key checks that need the fully merged config.
func validate(cfg *Config) error {
	if cfg.SSOEnabled && len(cfg.SSOSecret) == 0 {
		return fmt.Errorf("GFTP_SSO_SECRET must be set when GFTP_SSO_ENABLED is true")
	}
	if cfg.S3Enabled {
		switch {
		case cfg.S3Endpoint == "":
			return fmt.Errorf("GFTP_S3_ENDPOINT must be set when GFTP_S3_ENABLED is true")
		case !strings.HasPrefix(cfg.S3Endpoint, "http://") && !strings.HasPrefix(cfg.S3Endpoint, "https://"):
			return fmt.Errorf("GFTP_S3_ENDPOINT must include a scheme (http:// or https://), got %q", cfg.S3Endpoint)
		case cfg.S3Bucket == "":
			return fmt.Errorf("GFTP_S3_BUCKET must be set when GFTP_S3_ENABLED is true")
		case cfg.S3AccessKey == "":
			return fmt.Errorf("GFTP_S3_ACCESS_KEY must be set when GFTP_S3_ENABLED is true")
		case cfg.S3SecretKey == "":
			return fmt.Errorf("GFTP_S3_SECRET_KEY must be set when GFTP_S3_ENABLED is true")
		}
	}
	conn := &cfg.Settings.Connection
	if conn.LockHost && (conn.PresetHost == nil || *conn.PresetHost == "") {
		return fmt.Errorf("GFTP_CONNECTION_LOCK_HOST requires GFTP_CONNECTION_PRESET_HOST to be set")
	}
	if cfg.Settings.Branding.AppName == "" {
		cfg.Settings.Branding.AppName = "GoblinFTP"
	}
	return nil
}

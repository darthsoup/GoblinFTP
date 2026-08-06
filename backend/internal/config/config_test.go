package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/config"
)

// clearEnv unsets every GFTP_* variable, ambient ones included, relying on the
// documented "empty means unset" semantics.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if name, _, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(name, "GFTP_") {
			t.Setenv(name, "")
		}
	}
	for _, k := range config.Registry {
		t.Setenv(k.Env, "")
	}
	// Keep the stale-settings check from tripping over ambient /app/data.
	t.Setenv("GFTP_DATA_DIR", t.TempDir())
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load(nil)
	require.NoError(t, err)

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.NotEmpty(t, cfg.SessionSecret)
	assert.NotEmpty(t, cfg.DownloadTokenSecret)
	assert.Equal(t, int64(5*1024*1024), cfg.ChunkSize)
	assert.Equal(t, 1, cfg.MaxConcurrentUploads)
	assert.Equal(t, 5, cfg.LoginMaxAttempts)
	assert.Equal(t, 300, cfg.LoginCooldownSeconds)
	assert.Equal(t, 7200, cfg.SessionTTLSeconds)
	assert.False(t, cfg.SSOEnabled)

	assert.Equal(t, "json", cfg.LogFormat)
	assert.Empty(t, cfg.LogFile)
	assert.Equal(t, 10, cfg.LogFileMaxSizeMB)
	assert.Equal(t, 5, cfg.LogFileMaxBackups)
	assert.Equal(t, 0, cfg.LogFileMaxAgeDays)
	assert.True(t, cfg.FrontendLogEnabled)
	assert.False(t, cfg.MetricsEnabled)
	assert.Equal(t, "9091", cfg.MetricsPort)

	assert.Empty(t, cfg.Settings.UI.PageTitle, "empty page title falls back to branding.appName in the SPA")
	assert.Equal(t, []string{"ftp", "ftps", "sftp"}, cfg.Settings.Connection.AllowedTypes)
	assert.Equal(t, "en", cfg.Settings.Language)
	assert.False(t, cfg.Settings.Connection.DisableChmod)

	assert.Equal(t, "GoblinFTP", cfg.Settings.Branding.AppName)
	assert.Nil(t, cfg.Settings.Branding.LogoURL)
	assert.Nil(t, cfg.Settings.Branding.PrimaryColor)
	assert.False(t, cfg.Settings.Branding.HideAttribution)
}

func TestLoadBrandingFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_BRANDING_APP_NAME", "Acme Transfer")
	t.Setenv("GFTP_BRANDING_LOGO_URL", "https://acme.example/logo.svg")
	t.Setenv("GFTP_BRANDING_FAVICON_URL", "https://acme.example/favicon.ico")
	t.Setenv("GFTP_BRANDING_PRIMARY_COLOR", "#2563eb")
	t.Setenv("GFTP_BRANDING_PRIMARY_TEXT_COLOR", "#0b1220")
	t.Setenv("GFTP_BRANDING_HIDE_ATTRIBUTION", "true")

	cfg, err := config.Load(nil)
	require.NoError(t, err)

	b := cfg.Settings.Branding
	assert.Equal(t, "Acme Transfer", b.AppName)
	require.NotNil(t, b.LogoURL)
	assert.Equal(t, "https://acme.example/logo.svg", *b.LogoURL)
	require.NotNil(t, b.FaviconURL)
	assert.Equal(t, "https://acme.example/favicon.ico", *b.FaviconURL)
	require.NotNil(t, b.PrimaryColor)
	assert.Equal(t, "#2563eb", *b.PrimaryColor)
	require.NotNil(t, b.PrimaryTextColor)
	assert.Equal(t, "#0b1220", *b.PrimaryTextColor)
	assert.True(t, b.HideAttribution)
}

func TestLoadInvalidPrimaryColor(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_BRANDING_PRIMARY_COLOR", "blue")
	_, err := config.Load(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GFTP_BRANDING_PRIMARY_COLOR")
}

func TestLoadFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_PORT", "9090")
	t.Setenv("GFTP_LOG_LEVEL", "debug")
	t.Setenv("GFTP_SESSION_SECRET", "my-session-secret")
	t.Setenv("GFTP_DOWNLOAD_TOKEN_SECRET", "my-token-secret")
	t.Setenv("GFTP_SSO_ENABLED", "true")
	t.Setenv("GFTP_SSO_SECRET", "sso-secret")
	t.Setenv("GFTP_UPLOAD_CHUNK_SIZE", "1048576")
	t.Setenv("GFTP_UPLOAD_MAX_CONCURRENT", "7")
	t.Setenv("GFTP_LOGIN_MAX_ATTEMPTS", "3")
	t.Setenv("GFTP_LOGIN_COOLDOWN_SECONDS", "60")
	t.Setenv("GFTP_SESSION_TTL_SECONDS", "3600")
	t.Setenv("GFTP_UI_PAGE_TITLE", "MyFTP")
	t.Setenv("GFTP_LANGUAGE", "de")

	cfg, err := config.Load(nil)
	require.NoError(t, err)

	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, []byte("my-session-secret"), cfg.SessionSecret)
	assert.Equal(t, []byte("my-token-secret"), cfg.DownloadTokenSecret)
	assert.True(t, cfg.SSOEnabled)
	assert.Equal(t, int64(1048576), cfg.ChunkSize)
	assert.Equal(t, 7, cfg.MaxConcurrentUploads)
	assert.Equal(t, 3, cfg.LoginMaxAttempts)
	assert.Equal(t, 60, cfg.LoginCooldownSeconds)
	assert.Equal(t, 3600, cfg.SessionTTLSeconds)
	assert.Equal(t, "MyFTP", cfg.Settings.UI.PageTitle)
	assert.Equal(t, "de", cfg.Settings.Language)
}

func TestLoadSentryEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_SENTRY_ENVIRONMENT", "staging")
	t.Setenv("GFTP_SENTRY_RELEASE", "v1.2.3")
	t.Setenv("GFTP_SENTRY_SAMPLE_RATE", "0.25")

	cfg, err := config.Load(nil)
	require.NoError(t, err)
	assert.Equal(t, "staging", cfg.SentryEnvironment)
	assert.Equal(t, "v1.2.3", cfg.SentryRelease)
	assert.InDelta(t, 0.25, cfg.SentrySampleRate, 0.0001)
}

func TestLoadInvalidSentrySampleRate(t *testing.T) {
	for _, rate := range []string{"abc", "-0.1", "1.5"} {
		t.Run(rate, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("GFTP_SENTRY_SAMPLE_RATE", rate)
			_, err := config.Load(nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "GFTP_SENTRY_SAMPLE_RATE")
		})
	}
}

func TestLoadAutoGeneratesUniqueSecrets(t *testing.T) {
	clearEnv(t)
	cfg1, err := config.Load(nil)
	require.NoError(t, err)
	cfg2, err := config.Load(nil)
	require.NoError(t, err)

	assert.NotEqual(t, cfg1.SessionSecret, cfg2.SessionSecret)
	assert.NotEqual(t, cfg1.DownloadTokenSecret, cfg2.DownloadTokenSecret)
}

func TestLoadSSOEnabledWithoutSecretIsError(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_SSO_ENABLED", "true")
	_, err := config.Load(nil)
	assert.Error(t, err)
}

func setS3Env(t *testing.T) {
	t.Helper()
	t.Setenv("GFTP_S3_ENABLED", "true")
	t.Setenv("GFTP_S3_ENDPOINT", "http://localhost:9000")
	t.Setenv("GFTP_S3_BUCKET", "gftp-chunks")
	t.Setenv("GFTP_S3_ACCESS_KEY", "minioadmin")
	t.Setenv("GFTP_S3_SECRET_KEY", "minioadmin")
}

func TestLoadS3Defaults(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load(nil)
	require.NoError(t, err)

	assert.False(t, cfg.S3Enabled)
	assert.Equal(t, "us-east-1", cfg.S3Region)
	assert.True(t, cfg.S3UsePathStyle)
	assert.Equal(t, "gftp-uploads", cfg.S3Prefix)
	assert.Equal(t, 60, cfg.S3TimeoutSeconds)
}

func TestLoadS3FromEnv(t *testing.T) {
	clearEnv(t)
	setS3Env(t)
	t.Setenv("GFTP_S3_REGION", "eu-central-1")
	t.Setenv("GFTP_S3_USE_PATH_STYLE", "false")
	t.Setenv("GFTP_S3_PREFIX", "staging")
	t.Setenv("GFTP_S3_TIMEOUT_SECONDS", "120")

	cfg, err := config.Load(nil)
	require.NoError(t, err)

	assert.True(t, cfg.S3Enabled)
	assert.Equal(t, "http://localhost:9000", cfg.S3Endpoint)
	assert.Equal(t, "gftp-chunks", cfg.S3Bucket)
	assert.Equal(t, "eu-central-1", cfg.S3Region)
	assert.Equal(t, "minioadmin", cfg.S3AccessKey)
	assert.Equal(t, "minioadmin", cfg.S3SecretKey)
	assert.False(t, cfg.S3UsePathStyle)
	assert.Equal(t, "staging", cfg.S3Prefix)
	assert.Equal(t, 120, cfg.S3TimeoutSeconds)
}

func TestLoadS3EnabledMissingRequiredVarsIsError(t *testing.T) {
	for _, missing := range []string{
		"GFTP_S3_ENDPOINT", "GFTP_S3_BUCKET", "GFTP_S3_ACCESS_KEY", "GFTP_S3_SECRET_KEY",
	} {
		t.Run(missing, func(t *testing.T) {
			clearEnv(t)
			setS3Env(t)
			t.Setenv(missing, "")
			_, err := config.Load(nil)
			assert.Error(t, err)
		})
	}
}

func TestLoadS3EndpointWithoutSchemeIsError(t *testing.T) {
	clearEnv(t)
	setS3Env(t)
	t.Setenv("GFTP_S3_ENDPOINT", "localhost:9000")
	_, err := config.Load(nil)
	assert.Error(t, err)
}

func TestLoadConnectionPresetsFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_CONNECTION_PRESET_HOST", "ftp.example.com")
	t.Setenv("GFTP_CONNECTION_PRESET_PORT", "2121")
	t.Setenv("GFTP_CONNECTION_LOCK_HOST", "true")
	t.Setenv("GFTP_CONNECTION_PASSIVE_MODE", "false")

	cfg, err := config.Load(nil)
	require.NoError(t, err)

	require.NotNil(t, cfg.Settings.Connection.PresetHost)
	assert.Equal(t, "ftp.example.com", *cfg.Settings.Connection.PresetHost)
	require.NotNil(t, cfg.Settings.Connection.PresetPort)
	assert.Equal(t, 2121, *cfg.Settings.Connection.PresetPort)
	assert.True(t, cfg.Settings.Connection.LockHost)
	assert.False(t, cfg.Settings.Connection.PassiveMode)
}

func TestLoadConnectionPresetDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load(nil)
	require.NoError(t, err)
	assert.Nil(t, cfg.Settings.Connection.PresetHost)
	assert.Nil(t, cfg.Settings.Connection.PresetPort)
	assert.False(t, cfg.Settings.Connection.LockHost)
	assert.True(t, cfg.Settings.Connection.PassiveMode, "passive mode defaults to true")
}

func TestLoadLockHostRequiresPresetHost(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_CONNECTION_LOCK_HOST", "true")
	_, err := config.Load(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GFTP_CONNECTION_LOCK_HOST")
}

func TestLoadLoggingEnv(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr string
		check   func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "valid overrides",
			env: map[string]string{
				"GFTP_LOG_FORMAT":            "text",
				"GFTP_LOG_FILE":              "/tmp/gftp-test.log",
				"GFTP_LOG_FILE_MAX_SIZE_MB":  "25",
				"GFTP_LOG_FILE_MAX_BACKUPS":  "0",
				"GFTP_LOG_FILE_MAX_AGE_DAYS": "14",
				"GFTP_LOG_FRONTEND":          "false",
			},
			check: func(t *testing.T, cfg *config.Config) {
				assert.Equal(t, "text", cfg.LogFormat)
				assert.Equal(t, "/tmp/gftp-test.log", cfg.LogFile)
				assert.Equal(t, 25, cfg.LogFileMaxSizeMB)
				assert.Equal(t, 0, cfg.LogFileMaxBackups)
				assert.Equal(t, 14, cfg.LogFileMaxAgeDays)
				assert.False(t, cfg.FrontendLogEnabled)
			},
		},
		{
			name: "frontend log explicit true",
			env:  map[string]string{"GFTP_LOG_FRONTEND": "true"},
			check: func(t *testing.T, cfg *config.Config) {
				assert.True(t, cfg.FrontendLogEnabled)
			},
		},
		{name: "invalid format", env: map[string]string{"GFTP_LOG_FORMAT": "xml"}, wantErr: "GFTP_LOG_FORMAT"},
		{name: "invalid level", env: map[string]string{"GFTP_LOG_LEVEL": "verbose"}, wantErr: "GFTP_LOG_LEVEL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			cfg, err := config.Load(nil)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, cfg)
		})
	}
}

// ── Iframe embedding ─────────────────────────────────────────────────────────

func TestLoadFrameAncestorsDefaultsToDenied(t *testing.T) {
	clearEnv(t)

	cfg, err := config.Load(nil)
	require.NoError(t, err)
	assert.Empty(t, cfg.FrameAncestors)
	assert.False(t, cfg.EmbeddingEnabled())
}

func TestLoadFrameAncestorsAccepts(t *testing.T) {
	cases := map[string][]string{
		"single":             {"https://panel.example.com"},
		"multiple":           {"https://panel.example.com", "https://eu.panel.example.com:8443"},
		"wildcard subdomain": {"https://*.example.com"},
		// Compose service names and k8s short DNS have no dot at all; a regex
		// tight enough to look reasonable rejects these.
		"single label host": {"https://panel:8443"},
		"loopback http":     {"http://localhost:3000"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("GFTP_FRAME_ANCESTORS", strings.Join(want, " "))

			cfg, err := config.Load(nil)
			require.NoError(t, err)
			assert.Equal(t, want, cfg.FrameAncestors)
			assert.True(t, cfg.EmbeddingEnabled())
		})
	}
}

func TestLoadFrameAncestorsNormalizesAndDedupes(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_FRAME_ANCESTORS", "https://Panel.Example.com  https://panel.example.com")

	cfg, err := config.Load(nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://panel.example.com"}, cfg.FrameAncestors)
}

func TestLoadFrameAncestorsRejects(t *testing.T) {
	cases := map[string]string{
		"wildcard all":       "*",
		"wildcard scheme":    "https://*",
		"missing scheme":     "panel.example.com",
		"path":               "https://panel.example.com/embed",
		"trailing slash":     "https://panel.example.com/",
		"comma separated":    "https://a.example.com,https://b.example.com",
		"non loopback http":  "http://panel.example.com",
		"csp keyword":        "'self'",
		"credentials":        "https://user:pw@panel.example.com",
		"bare tld wildcard":  "https://*.com",
		"unsupported scheme": "ftp://panel.example.com",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("GFTP_FRAME_ANCESTORS", raw)

			_, err := config.Load(nil)
			require.Error(t, err, "%q must be rejected at startup", raw)
			assert.Contains(t, err.Error(), "GFTP_FRAME_ANCESTORS")
		})
	}
}

func TestLoadEmbedChromeless(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load(nil)
	require.NoError(t, err)
	assert.Equal(t, "auto", cfg.Settings.Embed.Chromeless)

	for _, v := range []string{"on", "off", "auto", "ON"} {
		clearEnv(t)
		t.Setenv("GFTP_EMBED_CHROMELESS", v)
		cfg, err := config.Load(nil)
		require.NoError(t, err)
		assert.Equal(t, strings.ToLower(v), cfg.Settings.Embed.Chromeless)
	}

	clearEnv(t)
	t.Setenv("GFTP_EMBED_CHROMELESS", "yes")
	_, err = config.Load(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GFTP_EMBED_CHROMELESS")
}

// ── Migration guards ─────────────────────────────────────────────────────────

func TestLoadBoolAcceptsParseBoolForms(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_SSO_ENABLED", "1")
	t.Setenv("GFTP_SSO_SECRET", "s")
	cfg, err := config.Load(nil)
	require.NoError(t, err)
	assert.True(t, cfg.SSOEnabled)
}

func TestLoadRejectsRenamedEnvNames(t *testing.T) {
	cases := map[string]string{
		"GFTP_APP_NAME":               "GFTP_BRANDING_APP_NAME",
		"GFTP_DISABLE_LOGIN_FORM":     "GFTP_LOGIN_FORM_DISABLED",
		"GFTP_SESSION_TTL_SECS":       "GFTP_SESSION_TTL_SECONDS",
		"GFTP_MAX_CONCURRENT_UPLOADS": "GFTP_UPLOAD_MAX_CONCURRENT",
	}
	for old, newName := range cases {
		t.Run(old, func(t *testing.T) {
			clearEnv(t)
			for name := range config.RenamedEnv() {
				t.Setenv(name, "")
			}
			t.Setenv(old, "some-value")
			_, err := config.Load(nil)
			require.Error(t, err, "a stale env name must fail startup, not be silently ignored")
			assert.Contains(t, err.Error(), newName)
		})
	}
}

func TestLoadRejectsRemovedEnvNames(t *testing.T) {
	for _, name := range []string{"GFTP_LOGIN_DISABLED_REDIRECT", "GFTP_SETTINGS_PATH"} {
		t.Run(name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(name, "some-value")
			_, err := config.Load(nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
		})
	}
}

func TestLoadRejectsStaleSettingsFile(t *testing.T) {
	clearEnv(t)
	dataDir := t.TempDir()
	t.Setenv("GFTP_DATA_DIR", dataDir)
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "settings.json"), []byte("{}"), 0o600))

	_, err := config.Load(nil)
	require.Error(t, err, "a mounted settings.json must fail startup instead of being silently ignored")
	assert.Contains(t, err.Error(), "settings.json")
	assert.Contains(t, err.Error(), "environment variables")
}

package config_test

import (
	"os"
	"testing"

	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"GFTP_PORT", "GFTP_LOG_LEVEL", "GFTP_SESSION_SECRET", "GFTP_DOWNLOAD_TOKEN_SECRET",
		"GFTP_SSO_ENABLED", "GFTP_SSO_SECRET", "GFTP_CHUNK_SIZE", "GFTP_MAX_CONCURRENT_UPLOADS",
		"GFTP_LOGIN_MAX_ATTEMPTS", "GFTP_LOGIN_COOLDOWN_SECS", "GFTP_SESSION_TTL_SECS",
		"GFTP_SENTRY_DSN", "GFTP_PAGE_TITLE", "GFTP_LOGIN_DISABLED_REDIRECT", "GFTP_SETTINGS_PATH",
		"GFTP_S3_ENABLED", "GFTP_S3_ENDPOINT", "GFTP_S3_BUCKET", "GFTP_S3_REGION",
		"GFTP_S3_ACCESS_KEY", "GFTP_S3_SECRET_KEY", "GFTP_S3_USE_PATH_STYLE",
		"GFTP_S3_PREFIX", "GFTP_S3_TIMEOUT_SECS",
		"GFTP_LOG_FORMAT", "GFTP_LOG_FILE", "GFTP_LOG_FILE_MAX_SIZE_MB",
		"GFTP_LOG_FILE_MAX_BACKUPS", "GFTP_LOG_FILE_MAX_AGE_DAYS", "GFTP_LOG_FRONTEND",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load(nil, "")
	require.NoError(t, err)

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.NotEmpty(t, cfg.SessionSecret)
	assert.NotEmpty(t, cfg.DownloadTokenSecret)
	assert.Equal(t, int64(5*1024*1024), cfg.ChunkSize)
	assert.Equal(t, 3, cfg.MaxConcurrentUploads)
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

	assert.Equal(t, "GoblinFTP", cfg.Settings.UI.PageTitle)
	assert.Equal(t, []string{"ftp", "sftp"}, cfg.Settings.Connection.AllowedTypes)
	assert.Equal(t, "en", cfg.Settings.Language)
	assert.False(t, cfg.Settings.Connection.DisableChmod)
	assert.Equal(t, 30, cfg.Settings.Connection.RequestTimeoutSeconds)
}

func TestLoadFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_PORT", "9090")
	t.Setenv("GFTP_LOG_LEVEL", "debug")
	t.Setenv("GFTP_SESSION_SECRET", "my-session-secret")
	t.Setenv("GFTP_DOWNLOAD_TOKEN_SECRET", "my-token-secret")
	t.Setenv("GFTP_SSO_ENABLED", "true")
	t.Setenv("GFTP_SSO_SECRET", "sso-secret")
	t.Setenv("GFTP_CHUNK_SIZE", "1048576")
	t.Setenv("GFTP_MAX_CONCURRENT_UPLOADS", "7")
	t.Setenv("GFTP_LOGIN_MAX_ATTEMPTS", "3")
	t.Setenv("GFTP_LOGIN_COOLDOWN_SECS", "60")
	t.Setenv("GFTP_SESSION_TTL_SECS", "3600")
	t.Setenv("GFTP_PAGE_TITLE", "MyFTP")

	cfg, err := config.Load(nil, "")
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
}

func TestLoadSettingsJSON(t *testing.T) {
	clearEnv(t)
	content := `{
		"language":"de",
		"ui":{"pageTitle":"Test FTP","showDotFiles":true,"showNavigationHistory":false,"helpUrl":null},
		"editor":{"openOnCreate":false,"allowedExtensions":["txt"],"disabled":true,"viewOnly":false},
		"connection":{"allowedTypes":["ftp"],"disableChmod":true,"requestTimeoutSeconds":60},
		"access":{"allowedClientAddresses":["127.0.0.1"],"deniedMessage":null,"postLogoutUrl":null}
	}`
	f, err := os.CreateTemp(".", "settings*.json")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	cfg, err := config.Load(nil, f.Name())
	require.NoError(t, err)

	assert.Equal(t, "de", cfg.Settings.Language)
	assert.Equal(t, "Test FTP", cfg.Settings.UI.PageTitle)
	assert.True(t, cfg.Settings.UI.ShowDotFiles)
	assert.True(t, cfg.Settings.Editor.Disabled)
	assert.Equal(t, []string{"ftp"}, cfg.Settings.Connection.AllowedTypes)
	assert.True(t, cfg.Settings.Connection.DisableChmod)
	assert.Equal(t, []string{"127.0.0.1"}, cfg.Settings.Access.AllowedClientAddresses)
}

func TestLoadPageTitleEnvOverridesSettings(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_PAGE_TITLE", "Override Title")

	content := `{"language":"en","ui":{"pageTitle":"From File","showDotFiles":false,"showNavigationHistory":true,"helpUrl":null},"editor":{"openOnCreate":false,"allowedExtensions":[],"disabled":false,"viewOnly":false},"connection":{"allowedTypes":["ftp","sftp"],"disableChmod":false,"requestTimeoutSeconds":30},"access":{"allowedClientAddresses":[],"deniedMessage":null,"postLogoutUrl":null}}`
	f, err := os.CreateTemp(".", "settings*.json")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	cfg, err := config.Load(nil, f.Name())
	require.NoError(t, err)
	assert.Equal(t, "Override Title", cfg.Settings.UI.PageTitle)
}

func TestLoadAutoGeneratesUniqueSecrets(t *testing.T) {
	clearEnv(t)
	cfg1, err := config.Load(nil, "")
	require.NoError(t, err)
	cfg2, err := config.Load(nil, "")
	require.NoError(t, err)

	assert.NotEqual(t, cfg1.SessionSecret, cfg2.SessionSecret)
	assert.NotEqual(t, cfg1.DownloadTokenSecret, cfg2.DownloadTokenSecret)
}

func TestLoadInvalidSettingsJSON(t *testing.T) {
	clearEnv(t)
	f, err := os.CreateTemp(".", "settings*.json")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	_, err = f.WriteString("not json")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = config.Load(nil, f.Name())
	assert.Error(t, err)
}

func TestLoadInvalidChunkSize(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_CHUNK_SIZE", "notanumber")
	_, err := config.Load(nil, "")
	assert.Error(t, err)
}

func TestLoadRejectsNonPositiveChunkSize(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_CHUNK_SIZE", "0")
	_, err := config.Load(nil, "")
	assert.Error(t, err)
}

func TestLoadInvalidMaxConcurrentUploads(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_MAX_CONCURRENT_UPLOADS", "0")
	_, err := config.Load(nil, "")
	assert.Error(t, err)
}

func TestLoadInvalidLoginMaxAttempts(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_LOGIN_MAX_ATTEMPTS", "0")
	_, err := config.Load(nil, "")
	assert.Error(t, err)
}

func TestLoadInvalidLoginCooldownSeconds(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_LOGIN_COOLDOWN_SECS", "0")
	_, err := config.Load(nil, "")
	assert.Error(t, err)
}

func TestLoadInvalidSessionTTL(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_SESSION_TTL_SECS", "-1")
	_, err := config.Load(nil, "")
	assert.Error(t, err)
}

func TestLoadSSOEnabledWithoutSecretIsError(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_SSO_ENABLED", "true")
	_, err := config.Load(nil, "")
	assert.Error(t, err)
}

func TestLoadMissingSettingsFileIsNotAnError(t *testing.T) {
	clearEnv(t)
	cfg, err := config.Load(nil, "./does-not-exist/settings.json")
	require.NoError(t, err)
	assert.Equal(t, "GoblinFTP", cfg.Settings.UI.PageTitle)
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
	cfg, err := config.Load(nil, "")
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
	t.Setenv("GFTP_S3_TIMEOUT_SECS", "120")

	cfg, err := config.Load(nil, "")
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
			_, err := config.Load(nil, "")
			assert.Error(t, err)
		})
	}
}

func TestLoadS3EndpointWithoutSchemeIsError(t *testing.T) {
	clearEnv(t)
	setS3Env(t)
	t.Setenv("GFTP_S3_ENDPOINT", "localhost:9000")
	_, err := config.Load(nil, "")
	assert.Error(t, err)
}

func TestLoadS3InvalidTimeoutIsError(t *testing.T) {
	clearEnv(t)
	t.Setenv("GFTP_S3_TIMEOUT_SECS", "0")
	_, err := config.Load(nil, "")
	assert.Error(t, err)
}

func TestLoadConnectionPresets(t *testing.T) {
	clearEnv(t)
	content := `{
		"connection":{"allowedTypes":["ftp"],"disableChmod":false,"requestTimeoutSeconds":30,
			"presetHost":"ftp.example.com","presetPort":2121,"lockHost":true,"passiveMode":false}
	}`
	f, err := os.CreateTemp(".", "settings*.json")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	cfg, err := config.Load(nil, f.Name())
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
	cfg, err := config.Load(nil, "")
	require.NoError(t, err)
	assert.Nil(t, cfg.Settings.Connection.PresetHost)
	assert.Nil(t, cfg.Settings.Connection.PresetPort)
	assert.False(t, cfg.Settings.Connection.LockHost)
	assert.True(t, cfg.Settings.Connection.PassiveMode, "passive mode defaults to true")
}

func TestLoadLockHostRequiresPresetHost(t *testing.T) {
	clearEnv(t)
	content := `{"connection":{"allowedTypes":["ftp"],"disableChmod":false,"requestTimeoutSeconds":30,"lockHost":true,"passiveMode":true}}`
	f, err := os.CreateTemp(".", "settings*.json")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = config.Load(nil, f.Name())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lockHost requires")
}

func TestLoadInvalidPresetPort(t *testing.T) {
	clearEnv(t)
	content := `{"connection":{"allowedTypes":["ftp"],"disableChmod":false,"requestTimeoutSeconds":30,"presetHost":"h","presetPort":70000,"passiveMode":true}}`
	f, err := os.CreateTemp(".", "settings*.json")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = config.Load(nil, f.Name())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "presetPort")
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
		{name: "non-numeric size", env: map[string]string{"GFTP_LOG_FILE_MAX_SIZE_MB": "abc"}, wantErr: "GFTP_LOG_FILE_MAX_SIZE_MB"},
		{name: "zero size", env: map[string]string{"GFTP_LOG_FILE_MAX_SIZE_MB": "0"}, wantErr: "GFTP_LOG_FILE_MAX_SIZE_MB"},
		{name: "negative backups", env: map[string]string{"GFTP_LOG_FILE_MAX_BACKUPS": "-1"}, wantErr: "GFTP_LOG_FILE_MAX_BACKUPS"},
		{name: "non-numeric age", env: map[string]string{"GFTP_LOG_FILE_MAX_AGE_DAYS": "abc"}, wantErr: "GFTP_LOG_FILE_MAX_AGE_DAYS"},
		{name: "negative age", env: map[string]string{"GFTP_LOG_FILE_MAX_AGE_DAYS": "-2"}, wantErr: "GFTP_LOG_FILE_MAX_AGE_DAYS"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			cfg, err := config.Load(nil, "")
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

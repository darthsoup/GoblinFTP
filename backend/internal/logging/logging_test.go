package logging_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/darthsoup/goblinftp/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitReturnsNonNilLogger(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "warning", "error", "invalid", ""} {
		logger, closeFn, err := logging.Init(logging.Options{Level: level})
		require.NoError(t, err, "level %q", level)
		assert.NotNil(t, logger, "expected non-nil logger for level %q", level)
		require.NotNil(t, closeFn)
		assert.NoError(t, closeFn(), "closer must be a no-op without a file sink")
	}
}

func TestInitCreatesLogFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "gftp.log")

	logger, closeFn, err := logging.Init(logging.Options{Level: "info", File: path})
	require.NoError(t, err)
	require.NotNil(t, logger)
	t.Cleanup(func() { _ = closeFn() })

	// The probe-open must have created the file eagerly.
	_, statErr := os.Stat(path)
	require.NoError(t, statErr)

	logger.Info("hello", "k", "v")
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Contains(t, string(data), `"msg":"hello"`)
	assert.NoError(t, closeFn())
}

func TestInitTextFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gftp.log")

	logger, closeFn, err := logging.Init(logging.Options{Level: "info", Format: "text", File: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeFn() })

	logger.Info("hello", "k", "v")
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Contains(t, string(data), "msg=hello")
	assert.NotContains(t, string(data), `"msg"`)
}

func TestInitUnwritableFileFailsFast(t *testing.T) {
	dir := t.TempDir()
	// A path whose parent is a regular file cannot be created.
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o640))

	_, _, err := logging.Init(logging.Options{File: filepath.Join(blocker, "gftp.log")})
	require.Error(t, err)
}

func TestSafeLogAttrsRedactsSensitiveKeys(t *testing.T) {
	attrs := logging.SafeLogAttrs(
		slog.String("password", "hunter2"),
		slog.String("username", "admin"),
		slog.String("secret_key", "abc123"),
		slog.String("token", "eyJhbGc"),
		slog.String("host", "example.com"),
		slog.String("credential", "value"),
	)
	require.Len(t, attrs, 6)

	assert.Equal(t, "[REDACTED]", attrs[0].Value.String())
	assert.Equal(t, "admin", attrs[1].Value.String())
	assert.Equal(t, "[REDACTED]", attrs[2].Value.String())
	assert.Equal(t, "[REDACTED]", attrs[3].Value.String())
	assert.Equal(t, "example.com", attrs[4].Value.String())
	assert.Equal(t, "[REDACTED]", attrs[5].Value.String())
}

func TestSafeLogAttrsCaseInsensitive(t *testing.T) {
	attrs := logging.SafeLogAttrs(
		slog.String("Password", "secret"),
		slog.String("API_KEY", "key-value"),
		slog.String("UserCredential", "cred"),
	)
	assert.Equal(t, "[REDACTED]", attrs[0].Value.String())
	assert.Equal(t, "[REDACTED]", attrs[1].Value.String())
	assert.Equal(t, "[REDACTED]", attrs[2].Value.String())
}

func TestSafeLogAttrsUsesSimpleSubstringMatching(t *testing.T) {
	attrs := logging.SafeLogAttrs(
		slog.String("API_KEY", "abc123"),
		slog.String("UserCredential", "cred"),
		slog.String("password", "hunter2"),
		slog.String("username", "admin"),
		slog.String("host", "example.com"),
		slog.String("goblin", "banana"),
		slog.String("keyboard", "qwerty"),
	)
	require.Len(t, attrs, 7)

	assert.Equal(t, "[REDACTED]", attrs[0].Value.String())
	assert.Equal(t, "[REDACTED]", attrs[1].Value.String())
	assert.Equal(t, "[REDACTED]", attrs[2].Value.String())
	assert.Equal(t, "admin", attrs[3].Value.String())
	assert.Equal(t, "example.com", attrs[4].Value.String())
	assert.Equal(t, "banana", attrs[5].Value.String())
	assert.Equal(t, "[REDACTED]", attrs[6].Value.String())
}

func TestSafeLogAttrsEmptyInput(t *testing.T) {
	attrs := logging.SafeLogAttrs()
	assert.Empty(t, attrs)
}

func TestSafeLogAttrsDoesNotModifyOriginal(t *testing.T) {
	original := slog.String("password", "hunter2")
	attrs := logging.SafeLogAttrs(original)

	assert.Equal(t, "hunter2", original.Value.String())
	assert.Equal(t, "[REDACTED]", attrs[0].Value.String())
}

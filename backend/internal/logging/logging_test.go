package logging_test

import (
	"log/slog"
	"testing"

	"github.com/darthsoup/goblinftp/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitReturnsNonNilLogger(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "warning", "error", "invalid", ""} {
		logger := logging.Init(level)
		assert.NotNil(t, logger, "expected non-nil logger for level %q", level)
	}
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

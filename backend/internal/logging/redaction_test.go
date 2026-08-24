package logging_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/darthsoup/goblinftp/internal/logging"
)

// Every caller passes the key "cause" and puts the secret in the value, so a
// key-only redaction layer provably redacted nothing.
func TestSafeLogAttrsRedactsSecretsInsideValues(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		leaked  string
		keepers []string
	}{
		{
			name:    "download token in a url",
			value:   `GET /api/files/download?token=eyJhbGciOi.SECRET&path=/a.txt failed`,
			leaked:  "eyJhbGciOi.SECRET",
			keepers: []string{"/api/files/download", "path=/a.txt"},
		},
		{
			name:    "sso token",
			value:   `sso login failed for sso=abc123def`,
			leaked:  "abc123def",
			keepers: []string{"sso login failed"},
		},
		{
			name:    "password in a connection string",
			value:   `dial failed: ftp://user:x@h?password=hunter2`,
			leaked:  "hunter2",
			keepers: []string{"dial failed"},
		},
		{
			name:    "csrf token",
			value:   `rejected: csrf=9f8e7d6c`,
			leaked:  "9f8e7d6c",
			keepers: []string{"rejected"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := logging.SafeLogAttrs(slog.String("cause", tt.value))
			out := got[0].Value.String()

			assert.NotContains(t, out, tt.leaked, "the secret must not reach the log")
			assert.Contains(t, out, "[REDACTED]")
			for _, keep := range tt.keepers {
				assert.Contains(t, out, keep, "the surrounding message must stay readable")
			}
		})
	}
}

// A remote path is the point of the access log and must survive untouched.
func TestSafeLogAttrsKeepsPaths(t *testing.T) {
	got := logging.SafeLogAttrs(slog.String("cause", "550 /reports/q1/keys.csv: no such file"))
	assert.Equal(t, "550 /reports/q1/keys.csv: no such file", got[0].Value.String())
}

// Key-based redaction still applies for attrs that name the secret directly.
func TestSafeLogAttrsStillRedactsByKey(t *testing.T) {
	got := logging.SafeLogAttrs(slog.String("password", "hunter2"))
	assert.Equal(t, "[REDACTED]", got[0].Value.String())
}

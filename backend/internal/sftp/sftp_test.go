package sftp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gsftp "github.com/darthsoup/goblinftp/internal/sftp"
)

func sftpHost(t *testing.T) string {
	t.Helper()
	h := os.Getenv("GFTP_TEST_SFTP_HOST")
	if h == "" {
		t.Skip("set GFTP_TEST_SFTP_HOST to run SFTP integration tests")
	}
	return h
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestDialBadHost(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	_, _, err := gsftp.Dial("127.0.0.1:1", "user", "pass", "", kh)
	assert.Error(t, err)
}

func TestDialIntegration(t *testing.T) {
	host := sftpHost(t)
	user := envOr("GFTP_TEST_SFTP_USER", "ftpuser")
	pass := envOr("GFTP_TEST_SFTP_PASS", "ftppass")
	kh := filepath.Join(t.TempDir(), "known_hosts")

	// First connect to an unknown host returns a trust-on-first-use prompt.
	c, prompt, err := gsftp.Dial(host, user, pass, "", kh)
	require.NoError(t, err)
	require.Nil(t, c)
	require.NotNil(t, prompt)

	// Accepting the shown fingerprint pins it and connects.
	c, prompt, err = gsftp.Dial(host, user, pass, prompt.Fingerprint, kh)
	require.NoError(t, err)
	require.Nil(t, prompt)
	require.NotNil(t, c)
	_ = c.Close()
}

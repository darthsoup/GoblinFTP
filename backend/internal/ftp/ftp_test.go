package ftp_test

import (
	"crypto/tls"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	gftp "github.com/darthsoup/goblinftp/internal/ftp"
)

// Integration tests require a live FTP server.
// Set GFTP_TEST_FTP_HOST=ftp.example.com:21 to run them.
func ftpHost(t *testing.T) string {
	t.Helper()
	h := os.Getenv("GFTP_TEST_FTP_HOST")
	if h == "" {
		t.Skip("set GFTP_TEST_FTP_HOST to run FTP integration tests")
	}
	return h
}

// Set GFTP_TEST_FTPS_HOST=localhost:2121 (just ftps-up) to run FTPS tests.
func ftpsHost(t *testing.T) string {
	t.Helper()
	h := os.Getenv("GFTP_TEST_FTPS_HOST")
	if h == "" {
		t.Skip("set GFTP_TEST_FTPS_HOST to run FTPS integration tests")
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
	_, err := gftp.Dial("127.0.0.1:1", "user", "pass", false, nil)
	assert.Error(t, err)
}

func TestDialIntegration(t *testing.T) {
	host := ftpHost(t)
	user := envOr("GFTP_TEST_FTP_USER", "ftpuser")
	pass := envOr("GFTP_TEST_FTP_PASS", "ftppass")

	c, err := gftp.Dial(host, user, pass, true, nil)
	assert.NoError(t, err)
	if err == nil {
		_ = c.Close()
	}
}

func TestDialFTPSIntegration(t *testing.T) {
	host := ftpsHost(t)
	user := envOr("GFTP_TEST_FTPS_USER", "ftpuser")
	pass := envOr("GFTP_TEST_FTPS_PASS", "ftppass")

	// Self-signed test-container cert — skip verification, like the app's
	// GFTP_FTP_TLS_INSECURE_SKIP_VERIFY escape hatch.
	tlsCfg := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	c, err := gftp.Dial(host, user, pass, true, tlsCfg)
	assert.NoError(t, err)
	if err == nil {
		_ = c.Close()
	}
}

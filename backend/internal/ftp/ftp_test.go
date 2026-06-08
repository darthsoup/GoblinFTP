package ftp_test

import (
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

func TestDialBadHost(t *testing.T) {
	_, err := gftp.Dial("127.0.0.1:1", "user", "pass", false, nil)
	assert.Error(t, err)
}

func TestDialIntegration(t *testing.T) {
	host := ftpHost(t)
	user := os.Getenv("GFTP_TEST_FTP_USER")
	pass := os.Getenv("GFTP_TEST_FTP_PASS")

	c, err := gftp.Dial(host, user, pass, true, nil)
	assert.NoError(t, err)
	if err == nil {
		_ = c.Close()
	}
}

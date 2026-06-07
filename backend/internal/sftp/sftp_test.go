package sftp_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestDialBadHost(t *testing.T) {
	_, err := gsftp.Dial("127.0.0.1:1", "user", "pass")
	assert.Error(t, err)
}

func TestDialIntegration(t *testing.T) {
	host := sftpHost(t)
	user := os.Getenv("GFTP_TEST_SFTP_USER")
	pass := os.Getenv("GFTP_TEST_SFTP_PASS")

	c, err := gsftp.Dial(host, user, pass)
	assert.NoError(t, err)
	if err == nil {
		_ = c.Close()
	}
}

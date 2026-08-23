package ftp

import (
	"errors"
	"testing"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// jlaffaye/ftp exposes no raw command, so SITE CHMOD is impossible and Chmod can
// only fail. Neither method connects, hence no GFTP_TEST_FTP_HOST gate.
func TestChmodUnsupported(t *testing.T) {
	c := &Client{}

	if c.SupportsChmod() {
		t.Error("SupportsChmod() = true, want false - FTP has no SITE CHMOD here")
	}
	if err := c.Chmod("/some/file", 0o644); !errors.Is(err, transfer.ErrPermissionsNotSupported) {
		t.Errorf("Chmod() error = %v, want ErrPermissionsNotSupported", err)
	}
}

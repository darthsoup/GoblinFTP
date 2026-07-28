// backend/internal/ftp/chmod_test.go
package ftp

import (
	"errors"
	"testing"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// The capability and the operation must agree: jlaffaye/ftp exposes no raw
// command, so there is no SITE CHMOD and Chmod can only fail. Advertising
// otherwise puts a control in the UI that always errors.
//
// Neither method touches the connection, so this needs no live server and is
// not gated by GFTP_TEST_FTP_HOST like the integration tests.
func TestChmodUnsupported(t *testing.T) {
	c := &Client{}

	if c.SupportsChmod() {
		t.Error("SupportsChmod() = true, want false — FTP has no SITE CHMOD here")
	}
	if err := c.Chmod("/some/file", 0o644); !errors.Is(err, transfer.ErrPermissionsNotSupported) {
		t.Errorf("Chmod() error = %v, want ErrPermissionsNotSupported", err)
	}
}

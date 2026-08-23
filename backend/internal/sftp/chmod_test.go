package sftp

import "testing"

// SFTP defines SETSTAT, so the capability is true; a server refusing one path
// surfaces as a normal Chmod error. No connection, hence no GFTP_TEST_SFTP_HOST gate.
func TestSupportsChmod(t *testing.T) {
	if !(&Client{}).SupportsChmod() {
		t.Error("SupportsChmod() = false, want true - SFTP implements SETSTAT")
	}
}

// backend/internal/sftp/chmod_test.go
package sftp

import "testing"

// SFTP defines SETSTAT, so the capability is true. A specific server may still
// refuse a specific path — that surfaces as a normal error from Chmod, not as a
// missing capability.
//
// SupportsChmod touches no connection, so this needs no live server and is not
// gated by GFTP_TEST_SFTP_HOST like the integration tests.
func TestSupportsChmod(t *testing.T) {
	if !(&Client{}).SupportsChmod() {
		t.Error("SupportsChmod() = false, want true — SFTP implements SETSTAT")
	}
}

package sftp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestHashedHostPinCannotBeRotatedByCaller(t *testing.T) {
	file := filepath.Join(t.TempDir(), "known_hosts")
	pinned, changed := testHostKey(t), testHostKey(t)
	entry := knownhosts.Line([]string{knownhosts.HashHostname(knownhosts.Normalize(testAddr))}, pinned)
	require.NoError(t, os.WriteFile(file, []byte(entry+"\n"), 0600))
	var res hostKeyResult
	verify, err := buildHostKeyCallback(testAddr, file, ssh.FingerprintSHA256(changed), &res)
	require.NoError(t, err)
	require.ErrorIs(t, verify(testAddr, testRemote, changed), errHostKeyHalt)
	require.True(t, res.mismatch)
	require.NoError(t, verify(testAddr, testRemote, pinned))
}

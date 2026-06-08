package sftp

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func testHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return key
}

var testRemote = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 22}

const testAddr = "host.example:22"

func TestHostKeyCallbackUnknownPrompts(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	key := testHostKey(t)

	var res hostKeyResult
	cb, err := buildHostKeyCallback(testAddr, kh, "", &res)
	require.NoError(t, err)

	err = cb(testAddr, testRemote, key)
	assert.ErrorIs(t, err, errHostKeyHalt)
	require.NotNil(t, res.prompt)
	assert.Equal(t, ssh.FingerprintSHA256(key), res.prompt.Fingerprint)
	assert.Equal(t, key.Type(), res.prompt.KeyType)
	assert.False(t, res.mismatch)

	// Nothing is pinned until the user accepts.
	data, _ := os.ReadFile(kh)
	assert.Empty(t, string(data))
}

func TestHostKeyCallbackAcceptPinsAndTrustsNextTime(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	key := testHostKey(t)

	var res hostKeyResult
	cb, err := buildHostKeyCallback(testAddr, kh, ssh.FingerprintSHA256(key), &res)
	require.NoError(t, err)
	require.NoError(t, cb(testAddr, testRemote, key)) // accepted → proceeds
	assert.Nil(t, res.prompt)

	// A fresh callback with no accept fingerprint now trusts the pinned key.
	var res2 hostKeyResult
	cb2, err := buildHostKeyCallback(testAddr, kh, "", &res2)
	require.NoError(t, err)
	assert.NoError(t, cb2(testAddr, testRemote, key))
	assert.Nil(t, res2.prompt)
	assert.False(t, res2.mismatch)
}

func TestHostKeyCallbackMismatch(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	pinned := testHostKey(t)
	imposter := testHostKey(t)

	var res hostKeyResult
	cb, err := buildHostKeyCallback(testAddr, kh, ssh.FingerprintSHA256(pinned), &res)
	require.NoError(t, err)
	require.NoError(t, cb(testAddr, testRemote, pinned)) // pin the real key

	// A different key for the same host is a possible MITM.
	var res2 hostKeyResult
	cb2, err := buildHostKeyCallback(testAddr, kh, "", &res2)
	require.NoError(t, err)
	err = cb2(testAddr, testRemote, imposter)
	assert.ErrorIs(t, err, errHostKeyHalt)
	assert.True(t, res2.mismatch)
	assert.Nil(t, res2.prompt)
}

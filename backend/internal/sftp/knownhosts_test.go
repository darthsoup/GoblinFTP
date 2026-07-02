package sftp

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	assert.False(t, res.prompt.Changed)

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
}

func TestHostKeyCallbackChangedKeyPrompts(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	pinned := testHostKey(t)
	changed := testHostKey(t)

	var res hostKeyResult
	cb, err := buildHostKeyCallback(testAddr, kh, ssh.FingerprintSHA256(pinned), &res)
	require.NoError(t, err)
	require.NoError(t, cb(testAddr, testRemote, pinned)) // pin the real key

	// A different key for the same host halts with a changed-key prompt showing
	// both fingerprints; nothing is replaced until the user re-trusts.
	var res2 hostKeyResult
	cb2, err := buildHostKeyCallback(testAddr, kh, "", &res2)
	require.NoError(t, err)
	err = cb2(testAddr, testRemote, changed)
	assert.ErrorIs(t, err, errHostKeyHalt)
	require.NotNil(t, res2.prompt)
	assert.True(t, res2.prompt.Changed)
	assert.Equal(t, ssh.FingerprintSHA256(changed), res2.prompt.Fingerprint)
	assert.Equal(t, ssh.FingerprintSHA256(pinned), res2.prompt.OldFingerprint)

	// The old pin is still honored — an accept fingerprint for the OLD key must
	// not authorize the new one.
	var res3 hostKeyResult
	cb3, err := buildHostKeyCallback(testAddr, kh, ssh.FingerprintSHA256(pinned), &res3)
	require.NoError(t, err)
	assert.ErrorIs(t, cb3(testAddr, testRemote, changed), errHostKeyHalt)
	require.NotNil(t, res3.prompt)
}

func TestHostKeyCallbackAcceptChangedReplacesPin(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	pinned := testHostKey(t)
	changed := testHostKey(t)

	var res hostKeyResult
	cb, err := buildHostKeyCallback(testAddr, kh, ssh.FingerprintSHA256(pinned), &res)
	require.NoError(t, err)
	require.NoError(t, cb(testAddr, testRemote, pinned))

	// Accepting the NEW fingerprint replaces the pin and proceeds.
	var res2 hostKeyResult
	cb2, err := buildHostKeyCallback(testAddr, kh, ssh.FingerprintSHA256(changed), &res2)
	require.NoError(t, err)
	assert.NoError(t, cb2(testAddr, testRemote, changed))
	assert.Nil(t, res2.prompt)

	// Exactly one entry remains and it trusts the new key silently…
	data, err := os.ReadFile(kh)
	require.NoError(t, err)
	assert.Len(t, strings.Split(strings.TrimSpace(string(data)), "\n"), 1)
	var res3 hostKeyResult
	cb3, err := buildHostKeyCallback(testAddr, kh, "", &res3)
	require.NoError(t, err)
	assert.NoError(t, cb3(testAddr, testRemote, changed))

	// …while the old key now triggers the changed-key prompt.
	var res4 hostKeyResult
	cb4, err := buildHostKeyCallback(testAddr, kh, "", &res4)
	require.NoError(t, err)
	assert.ErrorIs(t, cb4(testAddr, testRemote, pinned), errHostKeyHalt)
	require.NotNil(t, res4.prompt)
	assert.True(t, res4.prompt.Changed)
}

func TestReplaceKnownHostKeepsOtherEntries(t *testing.T) {
	kh := filepath.Join(t.TempDir(), "known_hosts")
	other := testHostKey(t)
	pinned := testHostKey(t)
	changed := testHostKey(t)
	require.NoError(t, appendKnownHost(kh, "other.example:22", other))
	require.NoError(t, appendKnownHost(kh, testAddr, pinned))

	require.NoError(t, replaceKnownHost(kh, testAddr, changed))

	// The other host's pin is untouched; testAddr now trusts only the new key.
	var res hostKeyResult
	cb, err := buildHostKeyCallback("other.example:22", kh, "", &res)
	require.NoError(t, err)
	assert.NoError(t, cb("other.example:22", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 2), Port: 22}, other))
	var res2 hostKeyResult
	cb2, err := buildHostKeyCallback(testAddr, kh, "", &res2)
	require.NoError(t, err)
	assert.NoError(t, cb2(testAddr, testRemote, changed))
}

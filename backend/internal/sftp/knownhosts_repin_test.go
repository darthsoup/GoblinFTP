package sftp

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func newHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signerKey, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return signerKey
}

// replaceKnownHost skipped |1|salt|hash entries, so confirming a changed key was
// a security no-op: the old key stayed pinned and the connection reported success.
func TestReplaceKnownHostRemovesHashedEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known_hosts")
	addr := "example.com:22"
	norm := knownhosts.Normalize(addr)

	oldKey := newHostKey(t)
	newKey := newHostKey(t)

	// A hashed entry, exactly as OpenSSH writes with HashKnownHosts=yes.
	hashed := knownhosts.HashHostname(norm)
	require.NoError(t, os.WriteFile(path,
		[]byte(knownhosts.Line([]string{hashed}, oldKey)+"\n"), 0o600))

	require.NoError(t, replaceKnownHost(path, addr, newKey))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	body := string(data)

	assert.NotContains(t, body, string(ssh.MarshalAuthorizedKey(oldKey))[:40],
		"the superseded key must be gone, not merely shadowed by a newer line")
	assert.Contains(t, body, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(newKey))))

	// The file must actually verify against the new key now.
	verify, err := knownhosts.New(path)
	require.NoError(t, err)
	assert.NoError(t, verify(addr, &fakeAddr{}, newKey), "the new key must verify")
	assert.Error(t, verify(addr, &fakeAddr{}, oldKey), "the old key must no longer verify")
}

func TestReplaceKnownHostRemovesPlainEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known_hosts")
	addr := "plain.example:22"
	norm := knownhosts.Normalize(addr)

	oldKey := newHostKey(t)
	newKey := newHostKey(t)
	require.NoError(t, os.WriteFile(path,
		[]byte(knownhosts.Line([]string{norm}, oldKey)+"\n"), 0o600))

	require.NoError(t, replaceKnownHost(path, addr, newKey))

	verify, err := knownhosts.New(path)
	require.NoError(t, err)
	assert.NoError(t, verify(addr, &fakeAddr{}, newKey))
	assert.Error(t, verify(addr, &fakeAddr{}, oldKey))
}

// A surviving marker line for the same host means the rewrite could not
// guarantee the old key is untrusted, so the dial has to fail loudly.
func TestReplaceKnownHostRefusesWhenMarkerSurvives(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known_hosts")
	addr := "marked.example:22"
	norm := knownhosts.Normalize(addr)

	oldKey := newHostKey(t)
	require.NoError(t, os.WriteFile(path,
		[]byte("@revoked "+norm+" "+strings.TrimSpace(string(ssh.MarshalAuthorizedKey(oldKey)))+"\n"), 0o600))

	err := replaceKnownHost(path, addr, newHostKey(t))
	require.Error(t, err, "a revoked marker must not be silently overridden by a re-pin")
}

// Other hosts' entries must survive a re-pin untouched.
func TestReplaceKnownHostLeavesOtherHostsAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known_hosts")
	otherAddr := "other.example:22"
	otherNorm := knownhosts.Normalize(otherAddr)
	otherKey := newHostKey(t)

	require.NoError(t, os.WriteFile(path,
		[]byte(knownhosts.Line([]string{otherNorm}, otherKey)+"\n"), 0o600))

	require.NoError(t, replaceKnownHost(path, "target.example:22", newHostKey(t)))

	verify, err := knownhosts.New(path)
	require.NoError(t, err)
	assert.NoError(t, verify(otherAddr, &fakeAddr{}, otherKey), "an unrelated host must stay trusted")
}

func TestHashedHostMatchesUsesTheLineOwnSalt(t *testing.T) {
	norm := knownhosts.Normalize("salted.example:22")
	hashed := knownhosts.HashHostname(norm)

	assert.True(t, hashedHostMatches(hashed, norm))
	assert.False(t, hashedHostMatches(hashed, knownhosts.Normalize("other.example:22")))

	// HashHostname picks a fresh salt every call, which is exactly why comparing
	// two of its outputs can never work.
	assert.NotEqual(t, hashed, knownhosts.HashHostname(norm))
}

// fakeAddr is a stand-in remote address; knownhosts only reads its String().
type fakeAddr struct{}

func (a *fakeAddr) Network() string { return "tcp" }
func (a *fakeAddr) String() string  { return "203.0.113.1:22" }

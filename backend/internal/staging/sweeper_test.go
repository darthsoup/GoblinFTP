package staging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validID = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// mkUpload creates an upload directory with one chunk, aged by the given amount.
func mkUpload(t *testing.T, root, id string, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(root, id)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "0000"), []byte("data"), 0o600))
	when := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(dir, when, when))
	return dir
}

// TestSweepReclaimsStaleUpload is the leak this closes: handlers clean up on
// commit and abort, but an abandoned reservation (or any upload reserved before
// a restart, since sessions are in-memory) was never collected.
func TestSweepReclaimsStaleUpload(t *testing.T) {
	root := t.TempDir()
	dir := mkUpload(t, root, validID, 48*time.Hour)

	newSweeper(root, discardLogger(), nil).Sweep()

	assert.NoDirExists(t, dir, "a stale upload directory must be reclaimed")
}

func TestSweepKeepsRecentUpload(t *testing.T) {
	root := t.TempDir()
	dir := mkUpload(t, root, validID, time.Minute)

	newSweeper(root, discardLogger(), nil).Sweep()

	assert.DirExists(t, dir, "an upload in progress must never be reclaimed")
}

// TestSweepLeavesForeignEntriesAlone is the safety property. GFTP_DATA_DIR also
// holds known_hosts and themes/, so a guard that is too loose deletes operator
// data.
func TestSweepLeavesForeignEntriesAlone(t *testing.T) {
	root := t.TempDir()
	old := time.Now().Add(-72 * time.Hour)

	knownHosts := filepath.Join(root, "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte("host ssh-rsa AAAA"), 0o600))
	require.NoError(t, os.Chtimes(knownHosts, old, old))

	themes := filepath.Join(root, "themes", "acme")
	require.NoError(t, os.MkdirAll(themes, 0o750))
	require.NoError(t, os.Chtimes(filepath.Join(root, "themes"), old, old))

	// Old, UUID-named, but holds something that is not a chunk.
	notOurs := filepath.Join(root, "6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	require.NoError(t, os.MkdirAll(notOurs, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(notOurs, "important.db"), []byte("x"), 0o600))
	require.NoError(t, os.Chtimes(notOurs, old, old))

	newSweeper(root, discardLogger(), nil).Sweep()

	assert.FileExists(t, knownHosts, "known_hosts must survive")
	assert.DirExists(t, themes, "theme assets must survive")
	assert.DirExists(t, notOurs, "a UUID directory that is not a chunk store must survive")
}

func TestSweeperCloseIsIdempotent(t *testing.T) {
	s := NewSweeper(t.TempDir(), discardLogger(), nil)
	s.Close()
	s.Close()
}

// TestSweepSkipsUploadStillReferenced is the guard against destroying a user's
// work: the age check alone is not enough, because the session TTL has no upper
// bound and a retried chunk (os.Create truncates in place) does not refresh the
// directory mtime.
func TestSweepSkipsUploadStillReferenced(t *testing.T) {
	root := t.TempDir()
	dir := mkUpload(t, root, validID, 72*time.Hour)

	inUse := func(id string) bool { return id == validID }
	newSweeper(root, discardLogger(), inUse).Sweep()

	assert.DirExists(t, dir, "an upload a live session still references must survive")

	newSweeper(root, discardLogger(), func(string) bool { return false }).Sweep()
	assert.NoDirExists(t, dir, "once nothing references it, the next pass reclaims it")
}

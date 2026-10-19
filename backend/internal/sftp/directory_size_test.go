package sftp

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestDirectorySizeIntegration(t *testing.T) {
	host := os.Getenv("GFTP_TEST_SFTP_HOST")
	if host == "" {
		t.Skip("set GFTP_TEST_SFTP_HOST to run SFTP integration tests")
	}
	user, pass := os.Getenv("GFTP_TEST_SFTP_USER"), os.Getenv("GFTP_TEST_SFTP_PASS")
	if user == "" {
		user = "ftpuser"
	}
	if pass == "" {
		pass = "ftppass"
	}
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	_, prompt, err := Dial(host, user, pass, "", knownHosts)
	require.NoError(t, err)
	require.NotNil(t, prompt)
	client, prompt, err := Dial(host, user, pass, prompt.Fingerprint, knownHosts)
	require.NoError(t, err)
	require.Nil(t, prompt)
	t.Cleanup(func() { _ = client.Close() })
	base := os.Getenv("GFTP_TEST_SFTP_BASE")
	if base == "" {
		base = "/upload"
	}
	root := path.Join(base, "gftp-directory-size-"+uuid.NewString())
	require.NoError(t, client.MakeDir(root))
	t.Cleanup(func() {
		_ = client.sftp.Remove(path.Join(root, "file-link"))
		_ = client.sftp.Remove(path.Join(root, "dir-link"))
		assert.NoError(t, client.Delete(root))
	})
	for _, dir := range []string{"child", ".hidden-dir", "empty"} {
		require.NoError(t, client.MakeDir(path.Join(root, dir)))
	}
	for name, content := range map[string]string{
		"visible.txt":             "abcde",
		".hidden":                 "xyz",
		"child/nested.txt":        "12345678",
		".hidden-dir/.deeper.txt": "1234",
	} {
		require.NoError(t, client.Upload(path.Join(root, name), strings.NewReader(content)))
	}
	require.NoError(t, client.sftp.Symlink(path.Join(root, "visible.txt"), path.Join(root, "file-link")))
	require.NoError(t, client.sftp.Symlink(path.Join(root, "child"), path.Join(root, "dir-link")))
	for _, name := range []string{"file-link", "dir-link"} {
		info, err := client.Lstat(path.Join(root, name))
		require.NoError(t, err)
		assert.True(t, info.IsSymlink)
	}
	result, err := transfer.CalculateDirectorySize(context.Background(), client, root)
	require.NoError(t, err)
	assert.Equal(t, int64(20), result.Bytes)
	assert.Equal(t, 4, result.FileCount)
	assert.Equal(t, 3, result.DirectoryCount)
	assert.True(t, result.Complete)
	assert.Zero(t, result.SkippedCount)

	_, err = transfer.CalculateDirectorySize(context.Background(), client, path.Join(root, "dir-link"))
	require.ErrorIs(t, err, transfer.ErrInvalidType)
	empty, err := transfer.CalculateDirectorySize(context.Background(), client, path.Join(root, "empty"))
	require.NoError(t, err)
	assert.True(t, empty.Complete)
	assert.Zero(t, empty.Bytes)
	assert.Zero(t, empty.FileCount)
	assert.Zero(t, empty.DirectoryCount)
	assert.False(t, empty.HasEntries)

	require.NoError(t, client.Upload(path.Join(root, "visible.txt"), strings.NewReader(strings.Repeat("x", 50))))
	updated, err := transfer.CalculateDirectorySize(context.Background(), client, root)
	require.NoError(t, err)
	assert.Equal(t, int64(65), updated.Bytes)
	assert.True(t, updated.Complete)
	assert.True(t, updated.HasEntries)

	entries, truncated, err := client.ReadDirContext(context.Background(), root, 2)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.True(t, truncated)
	require.NoError(t, client.Ping())

	linksOnly := path.Join(root, "links-only")
	require.NoError(t, client.MakeDir(linksOnly))
	link := path.Join(linksOnly, "link")
	require.NoError(t, client.sftp.Symlink(path.Join(root, "visible.txt"), link))
	t.Cleanup(func() { _ = client.sftp.Remove(link) })
	onlyLinks, err := transfer.CalculateDirectorySize(context.Background(), client, linksOnly)
	require.NoError(t, err)
	assert.True(t, onlyLinks.Complete)
	assert.True(t, onlyLinks.HasEntries)
	assert.Zero(t, onlyLinks.Bytes)
	assert.Zero(t, onlyLinks.FileCount)
}

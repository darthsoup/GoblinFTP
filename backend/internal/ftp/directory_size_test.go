package ftp_test

import (
	"context"
	"crypto/tls"
	"path"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gftp "github.com/darthsoup/goblinftp/internal/ftp"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestDirectorySizeIntegration(t *testing.T) {
	testDirectorySize(t, ftpHost(t), envOr("GFTP_TEST_FTP_USER", "ftpuser"), envOr("GFTP_TEST_FTP_PASS", "ftppass"), nil)
}

func TestDirectorySizeFTPSIntegration(t *testing.T) {
	testDirectorySize(t, ftpsHost(t), envOr("GFTP_TEST_FTPS_USER", "ftpuser"), envOr("GFTP_TEST_FTPS_PASS", "ftppass"), &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
}

func testDirectorySize(t *testing.T, host, user, password string, tlsConfig *tls.Config) {
	t.Helper()
	client, err := gftp.Dial(host, user, password, true, tlsConfig)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	base, err := client.WorkingDir()
	require.NoError(t, err)
	root := path.Join(base, "gftp-directory-size-"+uuid.NewString())
	require.NoError(t, client.MakeDir(root))
	t.Cleanup(func() {
		for _, name := range []string{"visible.txt", ".hidden", "child/nested.txt", ".hidden-dir/.deeper.txt"} {
			_ = client.Delete(path.Join(root, name))
		}
		for _, name := range []string{"child", ".hidden-dir", "empty"} {
			_ = client.Delete(path.Join(root, name))
		}
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
	result, err := transfer.CalculateDirectorySize(context.Background(), client, root)
	require.NoError(t, err)
	assert.Equal(t, int64(20), result.Bytes)
	assert.Equal(t, 4, result.FileCount)
	assert.Equal(t, 3, result.DirectoryCount)
	assert.True(t, result.Complete)
	assert.Zero(t, result.SkippedCount)

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
}

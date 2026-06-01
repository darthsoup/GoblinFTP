// backend/internal/transfer/chunk_test.go
package transfer_test

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUpload(t *testing.T) {
	dir := t.TempDir()
	meta, err := transfer.NewUpload(dir, "/remote/file.txt", 3, 1024)
	require.NoError(t, err)
	assert.NotEmpty(t, meta.ID)
	assert.Equal(t, "/remote/file.txt", meta.Destination)
	assert.Equal(t, 3, meta.TotalChunks)
	assert.Equal(t, int64(1024), meta.ChunkSize)
	_, statErr := os.Stat(dir + "/" + meta.ID)
	assert.NoError(t, statErr)
}

func TestWriteChunk(t *testing.T) {
	dir := t.TempDir()
	meta, err := transfer.NewUpload(dir, "/f.txt", 2, 5)
	require.NoError(t, err)

	require.NoError(t, transfer.WriteChunk(dir, meta.ID, 0, strings.NewReader("hello")))
	require.NoError(t, transfer.WriteChunk(dir, meta.ID, 1, strings.NewReader("world")))
}

func TestWriteChunk_InvalidID(t *testing.T) {
	dir := t.TempDir()
	err := transfer.WriteChunk(dir, "not-a-uuid", 0, strings.NewReader("x"))
	assert.Error(t, err)
}

func TestAssembleReader(t *testing.T) {
	dir := t.TempDir()
	meta, err := transfer.NewUpload(dir, "/f.txt", 2, 5)
	require.NoError(t, err)

	require.NoError(t, transfer.WriteChunk(dir, meta.ID, 0, strings.NewReader("hello")))
	require.NoError(t, transfer.WriteChunk(dir, meta.ID, 1, strings.NewReader("world")))

	rc, err := transfer.AssembleReader(dir, meta.ID, 2)
	require.NoError(t, err)
	defer rc.Close()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "helloworld", string(data))
}

func TestAssembleReader_MissingChunk(t *testing.T) {
	dir := t.TempDir()
	meta, err := transfer.NewUpload(dir, "/f.txt", 2, 5)
	require.NoError(t, err)

	// Write only chunk 0, not chunk 1.
	require.NoError(t, transfer.WriteChunk(dir, meta.ID, 0, strings.NewReader("hello")))

	_, err = transfer.AssembleReader(dir, meta.ID, 2)
	assert.Error(t, err)
}

func TestAssembleReader_InvalidID(t *testing.T) {
	dir := t.TempDir()
	_, err := transfer.AssembleReader(dir, "not-a-uuid", 1)
	assert.Error(t, err)
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	meta, err := transfer.NewUpload(dir, "/f.txt", 1, 5)
	require.NoError(t, err)

	require.NoError(t, transfer.Cleanup(dir, meta.ID))
	_, statErr := os.Stat(dir + "/" + meta.ID)
	assert.True(t, os.IsNotExist(statErr))
}

func TestCleanup_InvalidID(t *testing.T) {
	dir := t.TempDir()
	err := transfer.Cleanup(dir, "not-a-uuid")
	assert.Error(t, err)
}

// backend/internal/staging/local_test.go
package staging_test

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/darthsoup/goblinftp/internal/staging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests mirror internal/transfer/chunk_test.go against the ChunkStore
// interface, proving LocalStore preserves the original disk semantics.

func TestLocalNewUpload(t *testing.T) {
	dir := t.TempDir()
	store := staging.NewLocalStore(dir)
	meta, err := store.NewUpload(t.Context(), "/remote/file.txt", 3, 1024)
	require.NoError(t, err)
	assert.NotEmpty(t, meta.ID)
	assert.Equal(t, "/remote/file.txt", meta.Destination)
	assert.Equal(t, 3, meta.TotalChunks)
	assert.Equal(t, int64(1024), meta.ChunkSize)
	_, statErr := os.Stat(dir + "/" + meta.ID)
	assert.NoError(t, statErr)
}

func TestLocalWriteChunk(t *testing.T) {
	store := staging.NewLocalStore(t.TempDir())
	meta, err := store.NewUpload(t.Context(), "/f.txt", 2, 5)
	require.NoError(t, err)

	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("hello")))
	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 1, 5, strings.NewReader("world")))
}

func TestLocalWriteChunk_InvalidID(t *testing.T) {
	store := staging.NewLocalStore(t.TempDir())
	err := store.WriteChunk(t.Context(), "not-a-uuid", 0, 1, strings.NewReader("x"))
	assert.Error(t, err)
}

func TestLocalAssembleReader(t *testing.T) {
	store := staging.NewLocalStore(t.TempDir())
	meta, err := store.NewUpload(t.Context(), "/f.txt", 2, 5)
	require.NoError(t, err)

	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("hello")))
	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 1, 5, strings.NewReader("world")))

	rc, err := store.AssembleReader(t.Context(), meta.ID, 2)
	require.NoError(t, err)
	defer rc.Close()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "helloworld", string(data))
}

func TestLocalAssembleReader_MissingChunk(t *testing.T) {
	store := staging.NewLocalStore(t.TempDir())
	meta, err := store.NewUpload(t.Context(), "/f.txt", 2, 5)
	require.NoError(t, err)

	// Write only chunk 0, not chunk 1.
	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("hello")))

	_, err = store.AssembleReader(t.Context(), meta.ID, 2)
	assert.Error(t, err)
}

func TestLocalAssembleReader_InvalidID(t *testing.T) {
	store := staging.NewLocalStore(t.TempDir())
	_, err := store.AssembleReader(t.Context(), "not-a-uuid", 1)
	assert.Error(t, err)
}

func TestLocalCleanup(t *testing.T) {
	dir := t.TempDir()
	store := staging.NewLocalStore(dir)
	meta, err := store.NewUpload(t.Context(), "/f.txt", 1, 5)
	require.NoError(t, err)

	require.NoError(t, store.Cleanup(t.Context(), meta.ID))
	_, statErr := os.Stat(dir + "/" + meta.ID)
	assert.True(t, os.IsNotExist(statErr))
}

func TestLocalCleanup_InvalidID(t *testing.T) {
	store := staging.NewLocalStore(t.TempDir())
	err := store.Cleanup(t.Context(), "not-a-uuid")
	assert.Error(t, err)
}

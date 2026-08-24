package staging

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A full or read-only staging volume used to surface as a bare internal error,
// which the SPA treated as transient and retried five times per chunk.
func TestLocalStoreTagsUnwritableRootAsUnavailable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}
	root := t.TempDir()
	readOnly := filepath.Join(root, "staging")
	require.NoError(t, os.Mkdir(readOnly, 0o555))

	store := NewLocalStore(readOnly)
	_, err := store.NewUpload(context.Background(), "/dest.txt", 1, 1024)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnavailable,
		"an unwritable staging root is an operator problem, not an internal fault")
}

// A missing chunk means the upload is stale, so the caller has to start over.
// Reporting it as an internal fault invited an endless retry loop.
func TestAssembleReaderTagsMissingChunk(t *testing.T) {
	root := t.TempDir()
	store := NewLocalStore(root)

	meta, err := store.NewUpload(context.Background(), "/dest.txt", 2, 4)
	require.NoError(t, err)
	require.NoError(t, store.WriteChunk(context.Background(), meta.ID, 0, 4, strings.NewReader("abcd")))

	// Chunk 1 was never written.
	_, err = store.AssembleReader(context.Background(), meta.ID, 2)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrChunkMissing)
	assert.NotErrorIs(t, err, ErrUnavailable)
}

func TestLocalStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := NewLocalStore(root)
	ctx := context.Background()

	meta, err := store.NewUpload(ctx, "/dest.txt", 2, 3)
	require.NoError(t, err)
	require.NoError(t, store.WriteChunk(ctx, meta.ID, 0, 3, strings.NewReader("abc")))
	require.NoError(t, store.WriteChunk(ctx, meta.ID, 1, 2, strings.NewReader("de")))

	rc, err := store.AssembleReader(ctx, meta.ID, 2)
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()

	all, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "abcde", string(all))

	require.NoError(t, store.Cleanup(ctx, meta.ID))
	_, err = store.AssembleReader(ctx, meta.ID, 2)
	assert.True(t, errors.Is(err, ErrChunkMissing), "cleanup leaves nothing to assemble")
}

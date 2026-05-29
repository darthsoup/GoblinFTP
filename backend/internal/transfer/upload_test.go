// backend/internal/transfer/upload_test.go
package transfer_test

import (
	"testing"

	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadStoreCreateAndGet(t *testing.T) {
	store := transfer.NewUploadStore()
	id, meta := store.Create("/dest/file.txt", 3)
	assert.NotEmpty(t, id)
	assert.Equal(t, "/dest/file.txt", meta.Destination)
	assert.Equal(t, 3, meta.TotalChunks)
	assert.Equal(t, 0, meta.ReceivedChunks)

	got, ok := store.Get(id)
	require.True(t, ok)
	assert.Equal(t, meta, got)
}

func TestUploadStoreMarkReceived(t *testing.T) {
	store := transfer.NewUploadStore()
	id, _ := store.Create("/dest/file.txt", 3)

	store.MarkReceived(id)
	store.MarkReceived(id)

	got, ok := store.Get(id)
	require.True(t, ok)
	assert.Equal(t, 2, got.ReceivedChunks)
}

func TestUploadStoreDelete(t *testing.T) {
	store := transfer.NewUploadStore()
	id, _ := store.Create("/dest/file.txt", 2)
	store.Delete(id)
	_, ok := store.Get(id)
	assert.False(t, ok)
}

func TestUploadStoreGetMissing(t *testing.T) {
	store := transfer.NewUploadStore()
	_, ok := store.Get("nonexistent")
	assert.False(t, ok)
}

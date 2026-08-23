package staging

import (
	"context"
	"io"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// LocalStore stages chunks on local disk under dataDir, delegating to the
// original disk functions in internal/transfer.
type LocalStore struct {
	dataDir string
}

// NewLocalStore creates a LocalStore rooted at dataDir.
func NewLocalStore(dataDir string) *LocalStore {
	return &LocalStore{dataDir: dataDir}
}

func (s *LocalStore) NewUpload(_ context.Context, destination string, totalChunks int, chunkSize int64) (*transfer.UploadMeta, error) {
	return transfer.NewUpload(s.dataDir, destination, totalChunks, chunkSize)
}

func (s *LocalStore) WriteChunk(_ context.Context, uploadID string, index int, _ int64, r io.Reader) error {
	return transfer.WriteChunk(s.dataDir, uploadID, index, r)
}

func (s *LocalStore) AssembleReader(_ context.Context, uploadID string, totalChunks int) (io.ReadCloser, error) {
	return transfer.AssembleReader(s.dataDir, uploadID, totalChunks)
}

func (s *LocalStore) Cleanup(_ context.Context, uploadID string) error {
	return transfer.Cleanup(s.dataDir, uploadID)
}

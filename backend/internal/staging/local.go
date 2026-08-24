package staging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"syscall"

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
	meta, err := transfer.NewUpload(s.dataDir, destination, totalChunks, chunkSize)
	return meta, wrapDiskErr(err)
}

func (s *LocalStore) WriteChunk(_ context.Context, uploadID string, index int, _ int64, r io.Reader) error {
	return wrapDiskErr(transfer.WriteChunk(s.dataDir, uploadID, index, r))
}

func (s *LocalStore) AssembleReader(_ context.Context, uploadID string, totalChunks int) (io.ReadCloser, error) {
	rc, err := transfer.AssembleReader(s.dataDir, uploadID, totalChunks)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %w", ErrChunkMissing, err)
		}
		return nil, wrapDiskErr(err)
	}
	return rc, nil
}

func (s *LocalStore) Cleanup(_ context.Context, uploadID string) error {
	return wrapDiskErr(transfer.Cleanup(s.dataDir, uploadID))
}

// wrapDiskErr tags the failures an operator can fix. Without this a full or
// read-only staging volume surfaced as a bare 500 that the SPA then retried.
func wrapDiskErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT) ||
		errors.Is(err, syscall.EROFS) || errors.Is(err, syscall.EACCES) ||
		errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return err
}

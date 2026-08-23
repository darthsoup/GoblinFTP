// Package staging abstracts where upload chunks are staged before being streamed
// to the remote server: LocalStore on disk, optional S3Store in a bucket.
package staging

import (
	"context"
	"io"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// ChunkStore stages upload chunks between the browser and the remote server.
// Implementations must preserve the semantics of the disk functions in internal/transfer.
type ChunkStore interface {
	// NewUpload registers a new upload and returns its metadata (with a fresh ID).
	NewUpload(ctx context.Context, destination string, totalChunks int, chunkSize int64) (*transfer.UploadMeta, error)
	// WriteChunk stores one chunk; size is the multipart chunk length, which
	// implementations may ignore. Writing the same index twice overwrites.
	WriteChunk(ctx context.Context, uploadID string, index int, size int64, r io.Reader) error
	// AssembleReader returns a reader over all chunks in index order.
	// It fails if any chunk is missing. Caller must close the reader.
	AssembleReader(ctx context.Context, uploadID string, totalChunks int) (io.ReadCloser, error)
	// Cleanup removes all staged chunks for the upload.
	Cleanup(ctx context.Context, uploadID string) error
}

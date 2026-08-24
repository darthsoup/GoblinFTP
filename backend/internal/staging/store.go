// Package staging abstracts where upload chunks are staged before being streamed
// to the remote server: LocalStore on disk, optional S3Store in a bucket.
package staging

import (
	"context"
	"errors"
	"io"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// ErrUnavailable tags staging failures the operator can act on (a full volume, a
// read-only mount, an unreachable bucket) so they become a 503 rather than a 500.
var ErrUnavailable = errors.New("chunk storage unavailable")

// ErrChunkMissing marks an assembly that cannot find a chunk it was promised.
// That is a stale or already-cleaned upload, not an internal fault.
var ErrChunkMissing = errors.New("staged chunk missing")

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

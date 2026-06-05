// backend/internal/staging/store.go
//
// Package staging abstracts where upload chunks are staged before being
// assembled and streamed to the connected FTP/SFTP server. The default
// LocalStore keeps chunks on local disk (GFTP_DATA_DIR); the optional
// S3Store keeps them in an S3-compatible bucket.
package staging

import (
	"context"
	"io"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// ChunkStore stages upload chunks between the browser and the remote server.
// Implementations must preserve the semantics of the original disk-based
// functions in internal/transfer: NewUpload generates the upload ID,
// AssembleReader validates that all chunks exist before returning a reader,
// and the returned reader streams chunks in index order.
type ChunkStore interface {
	// NewUpload registers a new upload and returns its metadata (with a fresh ID).
	NewUpload(ctx context.Context, destination string, totalChunks int, chunkSize int64) (*transfer.UploadMeta, error)
	// WriteChunk stores one chunk. size is the chunk length in bytes
	// (from the multipart header); implementations may ignore it.
	// Writing the same index twice overwrites the previous data.
	WriteChunk(ctx context.Context, uploadID string, index int, size int64, r io.Reader) error
	// AssembleReader returns a reader over all chunks in index order.
	// It fails if any chunk is missing. Caller must close the reader.
	AssembleReader(ctx context.Context, uploadID string, totalChunks int) (io.ReadCloser, error)
	// Cleanup removes all staged chunks for the upload.
	Cleanup(ctx context.Context, uploadID string) error
}

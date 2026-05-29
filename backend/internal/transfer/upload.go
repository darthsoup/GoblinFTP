// backend/internal/transfer/upload.go
package transfer

import (
	"sync"

	"github.com/google/uuid"
)

// UploadMeta tracks the state of an in-progress chunked upload.
type UploadMeta struct {
	Destination    string
	TotalChunks    int
	ReceivedChunks int
}

// UploadStore is a thread-safe in-memory store for upload metadata.
type UploadStore struct {
	mu      sync.Mutex
	uploads map[string]*UploadMeta
}

func NewUploadStore() *UploadStore {
	return &UploadStore{uploads: make(map[string]*UploadMeta)}
}

// Create registers a new upload and returns its ID and a copy of the metadata.
func (s *UploadStore) Create(destination string, totalChunks int) (string, *UploadMeta) {
	id := uuid.NewString()
	meta := &UploadMeta{
		Destination: destination,
		TotalChunks: totalChunks,
	}
	s.mu.Lock()
	s.uploads[id] = meta
	s.mu.Unlock()
	return id, meta
}

// Get returns a pointer to the metadata for uploadID, or (nil, false) if not found.
func (s *UploadStore) Get(id string) (*UploadMeta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, ok := s.uploads[id]
	return meta, ok
}

// MarkReceived increments ReceivedChunks for uploadID.
func (s *UploadStore) MarkReceived(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if meta, ok := s.uploads[id]; ok {
		meta.ReceivedChunks++
	}
}

// Delete removes the upload entry from the store.
func (s *UploadStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.uploads, id)
}

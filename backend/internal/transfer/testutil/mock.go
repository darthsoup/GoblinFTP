package testutil

import (
	"io"
	"sync"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// MockClient is a transfer.Client where each method is a swappable function field.
// An unset field panics when called, intentionally, to catch missed test setup.
type MockClient struct {
	WorkingDirFn    func() (string, error)
	ListFn          func(path string) ([]transfer.FileInfo, error)
	StatFn          func(path string) (transfer.FileInfo, error)
	MakeDirFn       func(path string) error
	DeleteFn        func(path string) error
	RenameFn        func(src, dst string) error
	ChmodFn         func(path string, mode uint32) error
	SupportsChmodFn func() bool
	DownloadFn      func(path string) (io.ReadCloser, error)
	UploadFn        func(path string, r io.Reader) error
	PingFn          func() error
	CloseFn         func() error

	// closed may be written by an eviction goroutine, not just the test's own.
	// Read it through IsClosed.
	mu     sync.Mutex
	closed bool
}

func (m *MockClient) WorkingDir() (string, error)                   { return m.WorkingDirFn() }
func (m *MockClient) List(path string) ([]transfer.FileInfo, error) { return m.ListFn(path) }
func (m *MockClient) Stat(path string) (transfer.FileInfo, error)   { return m.StatFn(path) }
func (m *MockClient) MakeDir(path string) error                     { return m.MakeDirFn(path) }
func (m *MockClient) Delete(path string) error                      { return m.DeleteFn(path) }
func (m *MockClient) Rename(src, dst string) error                  { return m.RenameFn(src, dst) }
func (m *MockClient) Chmod(path string, mode uint32) error          { return m.ChmodFn(path, mode) }
func (m *MockClient) Download(path string) (io.ReadCloser, error)   { return m.DownloadFn(path) }
func (m *MockClient) Upload(path string, r io.Reader) error         { return m.UploadFn(path, r) }

// SupportsChmod defaults to true when SupportsChmodFn is unset, so tests that
// don't care about the capability keep working.
func (m *MockClient) SupportsChmod() bool {
	if m.SupportsChmodFn != nil {
		return m.SupportsChmodFn()
	}
	return true
}

// Ping defaults to alive (nil) when PingFn is unset, so existing tests
// that never deal with liveness keep working.
func (m *MockClient) Ping() error {
	if m.PingFn != nil {
		return m.PingFn()
	}
	return nil
}

func (m *MockClient) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	if m.CloseFn != nil {
		return m.CloseFn()
	}
	return nil
}

// IsClosed reports whether Close has been called.
func (m *MockClient) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

var _ transfer.Client = (*MockClient)(nil)

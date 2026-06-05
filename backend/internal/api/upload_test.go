package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/staging"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadSimple(t *testing.T) {
	var uploadedPath string
	var uploadedContent string

	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		UploadFn: func(path string, r io.Reader) error {
			uploadedPath = path
			data, _ := io.ReadAll(r)
			uploadedContent = string(data)
			return nil
		},
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("path", "/uploads/test.txt")
	part, _ := writer.CreateFormFile("file", "test.txt")
	_, _ = io.WriteString(part, "file contents here")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/files/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, "/uploads/test.txt", uploadedPath)
	assert.Equal(t, "file contents here", uploadedContent)
}

func TestUploadChunked(t *testing.T) {
	var assembled string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		UploadFn: func(path string, r io.Reader) error {
			data, _ := io.ReadAll(r)
			assembled = string(data)
			return nil
		},
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn))
	sess := connectAndGetSession(t, app)

	// Reserve
	reserveBody := `{"path":"/big.bin","totalChunks":2,"totalSize":10,"chunkSize":5}`
	reserveReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/reserve", strings.NewReader(reserveBody))
	reserveReq.Header.Set("Content-Type", "application/json")
	addSession(reserveReq, sess)
	reserveRec := httptest.NewRecorder()
	app.ServeHTTP(reserveRec, reserveReq)
	require.Equal(t, http.StatusOK, reserveRec.Code, "reserve: %s", reserveRec.Body.String())

	var reserveResp struct {
		Data struct {
			UploadID string `json:"uploadId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(reserveRec.Body.Bytes(), &reserveResp))
	uploadID := reserveResp.Data.UploadID
	require.NotEmpty(t, uploadID)

	// Send chunks
	sendChunk := func(n int, data string) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("uploadId", uploadID)
		_ = writer.WriteField("chunkIndex", fmt.Sprintf("%d", n))
		part, _ := writer.CreateFormFile("chunk", "chunk")
		_, _ = io.WriteString(part, data)
		writer.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/files/upload/chunk", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		addSession(req, sess)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "chunk %d: %s", n, rec.Body.String())
	}
	sendChunk(0, "hello")
	sendChunk(1, "world")

	// Commit
	commitBody := fmt.Sprintf(`{"uploadId":%q}`, uploadID)
	commitReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/commit", strings.NewReader(commitBody))
	commitReq.Header.Set("Content-Type", "application/json")
	addSession(commitReq, sess)
	commitRec := httptest.NewRecorder()
	app.ServeHTTP(commitRec, commitReq)
	require.Equal(t, http.StatusOK, commitRec.Code, "commit: %s", commitRec.Body.String())
	assert.Equal(t, "helloworld", assembled)
}

// memChunkStore is an in-memory staging.ChunkStore proving the upload
// handlers are agnostic to the staging backend.
type memChunkStore struct {
	mu       sync.Mutex
	chunks   map[string]map[int][]byte
	cleaned  []string
	writeErr error
}

func newMemChunkStore() *memChunkStore {
	return &memChunkStore{chunks: make(map[string]map[int][]byte)}
}

func (m *memChunkStore) NewUpload(_ context.Context, dest string, total int, size int64) (*transfer.UploadMeta, error) {
	return &transfer.UploadMeta{ID: uuid.NewString(), Destination: dest, TotalChunks: total, ChunkSize: size}, nil
}

func (m *memChunkStore) WriteChunk(_ context.Context, id string, idx int, _ int64, r io.Reader) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.chunks[id] == nil {
		m.chunks[id] = make(map[int][]byte)
	}
	m.chunks[id][idx] = data
	return nil
}

func (m *memChunkStore) AssembleReader(_ context.Context, id string, total int) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var buf bytes.Buffer
	for i := 0; i < total; i++ {
		data, ok := m.chunks[id][i]
		if !ok {
			return nil, fmt.Errorf("chunk %d missing", i)
		}
		buf.Write(data)
	}
	return io.NopCloser(&buf), nil
}

func (m *memChunkStore) Cleanup(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleaned = append(m.cleaned, id)
	delete(m.chunks, id)
	return nil
}

// runChunkedUpload drives the reserve→chunk→commit flow and returns the
// uploadID and the final commit response.
func runChunkedUpload(t *testing.T, app http.Handler, sess sessionCtx, chunks []string) (string, *httptest.ResponseRecorder) {
	t.Helper()

	reserveBody := fmt.Sprintf(`{"path":"/big.bin","totalChunks":%d,"totalSize":10,"chunkSize":5}`, len(chunks))
	reserveReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/reserve", strings.NewReader(reserveBody))
	reserveReq.Header.Set("Content-Type", "application/json")
	addSession(reserveReq, sess)
	reserveRec := httptest.NewRecorder()
	app.ServeHTTP(reserveRec, reserveReq)
	require.Equal(t, http.StatusOK, reserveRec.Code, "reserve: %s", reserveRec.Body.String())

	var reserveResp struct {
		Data struct {
			UploadID string `json:"uploadId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(reserveRec.Body.Bytes(), &reserveResp))
	uploadID := reserveResp.Data.UploadID
	require.NotEmpty(t, uploadID)

	for n, data := range chunks {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("uploadId", uploadID)
		_ = writer.WriteField("chunkIndex", fmt.Sprintf("%d", n))
		part, _ := writer.CreateFormFile("chunk", "chunk")
		_, _ = io.WriteString(part, data)
		writer.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/files/upload/chunk", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		addSession(req, sess)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "chunk %d: %s", n, rec.Body.String())
	}

	commitBody := fmt.Sprintf(`{"uploadId":%q}`, uploadID)
	commitReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/commit", strings.NewReader(commitBody))
	commitReq.Header.Set("Content-Type", "application/json")
	addSession(commitReq, sess)
	commitRec := httptest.NewRecorder()
	app.ServeHTTP(commitRec, commitReq)
	return uploadID, commitRec
}

func TestUploadChunkedWithCustomChunkStore(t *testing.T) {
	var assembled string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		UploadFn: func(path string, r io.Reader) error {
			data, _ := io.ReadAll(r)
			assembled = string(data)
			return nil
		},
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	store := newMemChunkStore()
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn), api.WithChunkStore(store))
	sess := connectAndGetSession(t, app)

	uploadID, commitRec := runChunkedUpload(t, app, sess, []string{"hello", "world"})
	require.Equal(t, http.StatusOK, commitRec.Code, "commit: %s", commitRec.Body.String())
	assert.Equal(t, "helloworld", assembled)
	assert.Contains(t, store.cleaned, uploadID, "staged chunks must be cleaned up after commit")
}

func TestUploadCommitFailureCleansUpChunks(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		UploadFn: func(path string, r io.Reader) error {
			_, _ = io.ReadAll(r)
			return fmt.Errorf("ftp server went away")
		},
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	store := newMemChunkStore()
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn), api.WithChunkStore(store))
	sess := connectAndGetSession(t, app)

	uploadID, commitRec := runChunkedUpload(t, app, sess, []string{"hello", "world"})
	assert.Equal(t, http.StatusInternalServerError, commitRec.Code)
	assert.Contains(t, commitRec.Body.String(), "ERR_OPERATION_FAILED")
	assert.Contains(t, store.cleaned, uploadID, "staged chunks must be cleaned up after a failed commit")

	// The upload is gone from the session — a second commit is a 404.
	commitBody := fmt.Sprintf(`{"uploadId":%q}`, uploadID)
	retryReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/commit", strings.NewReader(commitBody))
	retryReq.Header.Set("Content-Type", "application/json")
	addSession(retryReq, sess)
	retryRec := httptest.NewRecorder()
	app.ServeHTTP(retryRec, retryReq)
	assert.Equal(t, http.StatusNotFound, retryRec.Code)
	assert.Contains(t, retryRec.Body.String(), "ERR_UPLOAD_NOT_FOUND")
}

func TestUploadChunkStorageUnavailable(t *testing.T) {
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) {
		return &testutil.MockClient{
			WorkingDirFn: func() (string, error) { return "/", nil },
			ChmodFn:      func(string, uint32) error { return nil },
		}, nil
	}
	store := newMemChunkStore()
	store.writeErr = fmt.Errorf("%w: dial tcp: connection refused", staging.ErrUnavailable)
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(dialFn), api.WithChunkStore(store))
	sess := connectAndGetSession(t, app)

	reserveBody := `{"path":"/big.bin","totalChunks":1,"totalSize":5,"chunkSize":5}`
	reserveReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/reserve", strings.NewReader(reserveBody))
	reserveReq.Header.Set("Content-Type", "application/json")
	addSession(reserveReq, sess)
	reserveRec := httptest.NewRecorder()
	app.ServeHTTP(reserveRec, reserveReq)
	require.Equal(t, http.StatusOK, reserveRec.Code)

	var reserveResp struct {
		Data struct {
			UploadID string `json:"uploadId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(reserveRec.Body.Bytes(), &reserveResp))

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("uploadId", reserveResp.Data.UploadID)
	_ = writer.WriteField("chunkIndex", "0")
	part, _ := writer.CreateFormFile("chunk", "chunk")
	_, _ = io.WriteString(part, "hello")
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/files/upload/chunk", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "ERR_STORAGE_UNAVAILABLE")
}

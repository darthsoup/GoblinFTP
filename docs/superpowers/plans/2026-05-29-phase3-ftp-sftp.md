# Phase 3: FTP/SFTP Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire up the FTP and SFTP adapters behind a unified `transfer.Client` interface, implement all 18 API handlers (file ops, downloads, uploads, archives, system vars), and add chunked-upload support so GoblinFTP can actually talk to remote servers.

**Architecture:** A `transfer.Client` interface in `internal/transfer/client.go` is the only thing the HTTP handlers touch — FTP and SFTP adapters implement it independently. Download tokens are HMAC-signed opaque strings; chunked uploads store temp files in `{dataDir}/{uploadID}/{N:04d}` and are assembled on commit. Handler injection is via a `WithDial(fn)` functional option so tests never touch a real server.

**Tech Stack:** Go 1.26, Echo v4, `github.com/jlaffaye/ftp`, `github.com/pkg/sftp`, `golang.org/x/crypto/ssh`, stdlib `archive/zip` + `archive/tar`, `crypto/hmac` + `crypto/rand`, `encoding/base64`

---

## File Map

**Create:**
- `backend/internal/transfer/client.go` — `FileInfo` struct, `Client` interface (11 methods), 3 sentinel errors
- `backend/internal/transfer/token.go` — `IssueToken()`, `ParseToken()`, `ValidateToken()`
- `backend/internal/transfer/token_test.go`
- `backend/internal/transfer/upload.go` — `UploadMeta`, `NewUpload()`, `WriteChunk()`, `AssembleReader()`, `Cleanup()`
- `backend/internal/transfer/upload_test.go`
- `backend/internal/transfer/testutil/mock.go` — `MockClient` with function fields
- `backend/internal/ftp/ftp.go` — FTP adapter implementing `transfer.Client`
- `backend/internal/ftp/ftp_test.go` — unit + optional integration
- `backend/internal/sftp/sftp.go` — SFTP adapter implementing `transfer.Client`
- `backend/internal/sftp/sftp_test.go`
- `backend/internal/api/dial.go` — `defaultDial()` routing ftp:// vs sftp://
- `backend/internal/api/files.go` — `ListFiles`, `CreateDirectory`, `DeleteFiles`, `RenameFile`, `CopyFile`, `SetPermissions`
- `backend/internal/api/files_test.go`
- `backend/internal/api/download.go` — `IssueDownloadToken`, `DownloadFile`, `DownloadZip`
- `backend/internal/api/download_test.go`
- `backend/internal/api/upload.go` — `UploadSimple`, `UploadReserve`, `UploadChunk`, `UploadCommit`
- `backend/internal/api/upload_test.go`
- `backend/internal/api/archive.go` — `ExtractArchive`, `CreateZip`
- `backend/internal/api/archive_test.go`
- `backend/internal/api/system.go` — `SystemVars`
- `backend/internal/api/system_test.go`

**Modify:**
- `backend/internal/errors/errors.go` — add 5 error codes + HTTP status cases
- `backend/internal/config/config.go` — add `DataDir string`
- `backend/internal/api/handler.go` — add `DialFunc`, `HandlerOption`, `WithDial()`, `dataDir`, `dial` fields
- `backend/internal/api/router.go` — variadic `Register()`, move system/vars, add download-token route
- `backend/internal/api/connect.go` — full dial + session creation + `detectChmod`; `Disconnect` closes client
- `backend/internal/api/connect_test.go` — replace `TestConnectValidReturns501` with `TestConnectSuccess`
- `backend/internal/api/router_test.go` — variadic `newTestApp`, add `DataDir` to config helper

---

### Task 1: Error codes and DataDir config

**Files:**
- Modify: `backend/internal/errors/errors.go`
- Modify: `backend/internal/config/config.go`

- [ ] **Step 1: Add the 5 missing error codes to `errors.go`**

Open `backend/internal/errors/errors.go`. Add after the existing `const` block (after `ErrQuotaExceeded`):

```go
ErrConnectionTimeout     Code = "connection_timeout"
ErrPermissionsNotSupported Code = "permissions_not_supported"
ErrUploadNotFound        Code = "upload_not_found"
ErrInvalidToken          Code = "invalid_token"
ErrArchiveFormat         Code = "archive_format"
```

Then in the `HTTPStatus(c Code) int` switch, add before the `default` case:

```go
case ErrConnectionTimeout:
    return http.StatusGatewayTimeout
case ErrPermissionsNotSupported:
    return http.StatusUnprocessableEntity
case ErrUploadNotFound:
    return http.StatusNotFound
case ErrInvalidToken:
    return http.StatusUnauthorized
case ErrArchiveFormat:
    return http.StatusUnprocessableEntity
```

- [ ] **Step 2: Add `DataDir` to `config.go`**

In `backend/internal/config/config.go`, in the `Config` struct, add after `ChunkSize int64`:

```go
DataDir string
```

In the `Load()` function (or wherever env vars are read), add:

```go
cfg.DataDir = getEnv("GFTP_DATA_DIR", "/app/data")
```

where `getEnv` is whatever helper already exists (look for how `ChunkSize` / `DownloadTokenSecret` are loaded — follow the same pattern exactly).

- [ ] **Step 3: Run tests to verify no regressions**

```bash
cd backend && go test ./internal/errors/... ./internal/config/... -v
```

Expected: all tests pass (no new tests needed here — you're extending constants, not logic).

- [ ] **Step 4: Commit**

```bash
git add backend/internal/errors/errors.go backend/internal/config/config.go
git commit -m "feat: add Phase 3 error codes and DataDir config"
```

---

### Task 2: `transfer.Client` interface

**Files:**
- Create: `backend/internal/transfer/client.go`

- [ ] **Step 1: Create the file**

```go
// backend/internal/transfer/client.go
package transfer

import "errors"

// FileInfo represents a single remote filesystem entry.
type FileInfo struct {
	Name        string
	Size        int64
	IsDir       bool
	ModTime     int64  // Unix timestamp
	Permissions string // e.g. "drwxr-xr-x"
}

// Client is the unified interface that both FTP and SFTP adapters implement.
// All methods that accept a path expect an absolute path on the remote server.
type Client interface {
	// WorkingDir returns the current working directory.
	WorkingDir() (string, error)
	// List returns the contents of the given directory.
	List(path string) ([]FileInfo, error)
	// Stat returns info for a single path. On FTP, this lists the parent dir
	// and finds the entry by name.
	Stat(path string) (FileInfo, error)
	// MakeDir creates a directory (including parents if necessary).
	MakeDir(path string) error
	// Delete removes a file or empty directory.
	Delete(path string) error
	// Rename moves src to dst.
	Rename(src, dst string) error
	// Chmod sets permissions on the given path.
	// Returns ErrPermissionsNotSupported if the server does not support it.
	Chmod(path string, mode uint32) error
	// Download opens a reader for the given file. Caller must close it.
	Download(path string) (io.ReadCloser, error)
	// Upload streams from r into the given path, overwriting if it exists.
	Upload(path string, r io.Reader) error
	// Close terminates the underlying connection.
	Close() error
}

// Sentinel errors returned by adapters. Handlers check these with errors.Is.
var (
	ErrAuthFailed              = errors.New("auth failed")
	ErrConnectionFailed        = errors.New("connection failed")
	ErrPermissionsNotSupported = errors.New("permissions not supported")
)
```

Wait — add the missing `"io"` import. The full file is:

```go
// backend/internal/transfer/client.go
package transfer

import (
	"errors"
	"io"
)

// FileInfo represents a single remote filesystem entry.
type FileInfo struct {
	Name        string
	Size        int64
	IsDir       bool
	ModTime     int64  // Unix timestamp
	Permissions string // e.g. "drwxr-xr-x"
}

// Client is the unified interface that both FTP and SFTP adapters implement.
type Client interface {
	WorkingDir() (string, error)
	List(path string) ([]FileInfo, error)
	Stat(path string) (FileInfo, error)
	MakeDir(path string) error
	Delete(path string) error
	Rename(src, dst string) error
	Chmod(path string, mode uint32) error
	Download(path string) (io.ReadCloser, error)
	Upload(path string, r io.Reader) error
	Close() error
}

var (
	ErrAuthFailed              = errors.New("auth failed")
	ErrConnectionFailed        = errors.New("connection failed")
	ErrPermissionsNotSupported = errors.New("permissions not supported")
)
```

- [ ] **Step 2: Verify it compiles**

```bash
cd backend && go build ./internal/transfer/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add backend/internal/transfer/client.go
git commit -m "feat: add transfer.Client interface and sentinel errors"
```

---

### Task 3: MockClient for tests

**Files:**
- Create: `backend/internal/transfer/testutil/mock.go`

- [ ] **Step 1: Create the mock**

```go
// backend/internal/transfer/testutil/mock.go
package testutil

import (
	"io"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// MockClient is a transfer.Client where each method is a swappable function field.
// Any unset field panics when called — intentional, to catch missed setup in tests.
type MockClient struct {
	WorkingDirFn func() (string, error)
	ListFn       func(path string) ([]transfer.FileInfo, error)
	StatFn       func(path string) (transfer.FileInfo, error)
	MakeDirFn    func(path string) error
	DeleteFn     func(path string) error
	RenameFn     func(src, dst string) error
	ChmodFn      func(path string, mode uint32) error
	DownloadFn   func(path string) (io.ReadCloser, error)
	UploadFn     func(path string, r io.Reader) error
	CloseFn      func() error
	Closed       bool
}

func (m *MockClient) WorkingDir() (string, error)                { return m.WorkingDirFn() }
func (m *MockClient) List(path string) ([]transfer.FileInfo, error) { return m.ListFn(path) }
func (m *MockClient) Stat(path string) (transfer.FileInfo, error)   { return m.StatFn(path) }
func (m *MockClient) MakeDir(path string) error                     { return m.MakeDirFn(path) }
func (m *MockClient) Delete(path string) error                      { return m.DeleteFn(path) }
func (m *MockClient) Rename(src, dst string) error                  { return m.RenameFn(src, dst) }
func (m *MockClient) Chmod(path string, mode uint32) error          { return m.ChmodFn(path, mode) }
func (m *MockClient) Download(path string) (io.ReadCloser, error)   { return m.DownloadFn(path) }
func (m *MockClient) Upload(path string, r io.Reader) error         { return m.UploadFn(path, r) }
func (m *MockClient) Close() error {
	m.Closed = true
	if m.CloseFn != nil {
		return m.CloseFn()
	}
	return nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd backend && go build ./internal/transfer/testutil/...
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/transfer/testutil/mock.go
git commit -m "feat: add MockClient for transfer tests"
```

---

### Task 4: Download token

**Files:**
- Create: `backend/internal/transfer/token.go`
- Create: `backend/internal/transfer/token_test.go`

- [ ] **Step 1: Write the failing tests first**

```go
// backend/internal/transfer/token_test.go
package transfer_test

import (
	"testing"
	"time"

	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRoundTrip(t *testing.T) {
	secret := []byte("supersecret")
	sessionID := "sess-abc"
	path := "/some/path/file.txt"
	expiry := time.Now().Add(5 * time.Minute)

	tok, err := transfer.IssueToken(secret, sessionID, path, expiry)
	require.NoError(t, err)
	assert.NotEmpty(t, tok)

	gotSession, gotPath, err := transfer.ValidateToken(secret, tok)
	require.NoError(t, err)
	assert.Equal(t, sessionID, gotSession)
	assert.Equal(t, path, gotPath)
}

func TestTokenExpired(t *testing.T) {
	secret := []byte("supersecret")
	tok, err := transfer.IssueToken(secret, "s", "/f", time.Now().Add(-1*time.Second))
	require.NoError(t, err)

	_, _, err = transfer.ValidateToken(secret, tok)
	assert.ErrorIs(t, err, transfer.ErrTokenExpired)
}

func TestTokenTampered(t *testing.T) {
	secret := []byte("supersecret")
	tok, err := transfer.IssueToken(secret, "s", "/f", time.Now().Add(time.Minute))
	require.NoError(t, err)

	_, _, err = transfer.ValidateToken([]byte("wrong"), tok)
	assert.ErrorIs(t, err, transfer.ErrTokenInvalid)
}

func TestTokenMalformed(t *testing.T) {
	secret := []byte("supersecret")
	_, _, err := transfer.ValidateToken(secret, "notavalidtoken")
	assert.ErrorIs(t, err, transfer.ErrTokenInvalid)
}
```

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd backend && go test ./internal/transfer/... -run TestToken -v
```

Expected: compilation error — `transfer.IssueToken` / `ValidateToken` not defined.

- [ ] **Step 3: Implement `token.go`**

```go
// backend/internal/transfer/token.go
package transfer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrTokenExpired = errors.New("token expired")
	ErrTokenInvalid = errors.New("token invalid")
)

// IssueToken creates a signed download token.
// Format (before outer base64): sessionID:base64url(path):expiryUnix:hexHMAC
func IssueToken(secret []byte, sessionID, path string, expiry time.Time) (string, error) {
	encodedPath := base64.RawURLEncoding.EncodeToString([]byte(path))
	expiryStr := strconv.FormatInt(expiry.Unix(), 10)
	message := sessionID + ":" + encodedPath + ":" + expiryStr
	mac := computeHMAC(secret, message)
	raw := message + ":" + mac
	return base64.RawURLEncoding.EncodeToString([]byte(raw)), nil
}

// ValidateToken verifies the HMAC, checks expiry, and returns (sessionID, path).
func ValidateToken(secret []byte, token string) (sessionID, path string, err error) {
	rawBytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", "", ErrTokenInvalid
	}
	// Exactly 4 parts: sessionID, base64url(path), expiryUnix, hexHMAC
	parts := strings.SplitN(string(rawBytes), ":", 4)
	if len(parts) != 4 {
		return "", "", ErrTokenInvalid
	}
	sessionID, encodedPath, expiryStr, gotMAC := parts[0], parts[1], parts[2], parts[3]

	message := sessionID + ":" + encodedPath + ":" + expiryStr
	expectedMAC := computeHMAC(secret, message)
	if !hmac.Equal([]byte(gotMAC), []byte(expectedMAC)) {
		return "", "", ErrTokenInvalid
	}

	expiryUnix, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return "", "", ErrTokenInvalid
	}
	if time.Now().Unix() > expiryUnix {
		return "", "", ErrTokenExpired
	}

	pathBytes, err := base64.RawURLEncoding.DecodeString(encodedPath)
	if err != nil {
		return "", "", ErrTokenInvalid
	}
	return sessionID, string(pathBytes), nil
}

func computeHMAC(secret []byte, message string) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
```

- [ ] **Step 4: Run tests to confirm PASS**

```bash
cd backend && go test ./internal/transfer/... -run TestToken -v
```

Expected: 4 tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/transfer/token.go backend/internal/transfer/token_test.go
git commit -m "feat: add download token (HMAC-signed, base64url)"
```

---

### Task 5: Chunked upload state

**Files:**
- Create: `backend/internal/transfer/upload.go`
- Create: `backend/internal/transfer/upload_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// backend/internal/transfer/upload_test.go
package transfer_test

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUpload(t *testing.T) {
	dir := t.TempDir()
	meta, err := transfer.NewUpload(dir, "/remote/file.txt", 3, 1024)
	require.NoError(t, err)
	assert.NotEmpty(t, meta.ID)
	assert.Equal(t, "/remote/file.txt", meta.RemotePath)
	assert.Equal(t, 3, meta.TotalChunks)
	assert.Equal(t, int64(1024), meta.ChunkSize)
}

func TestWriteAndAssemble(t *testing.T) {
	dir := t.TempDir()
	meta, err := transfer.NewUpload(dir, "/remote/file.txt", 2, 5)
	require.NoError(t, err)

	err = transfer.WriteChunk(dir, meta.ID, 0, strings.NewReader("hello"))
	require.NoError(t, err)
	err = transfer.WriteChunk(dir, meta.ID, 1, strings.NewReader("world"))
	require.NoError(t, err)

	r, err := transfer.AssembleReader(dir, meta.ID, meta.TotalChunks)
	require.NoError(t, err)
	defer r.Close()

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "helloworld", string(data))
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	meta, err := transfer.NewUpload(dir, "/f", 1, 10)
	require.NoError(t, err)
	err = transfer.WriteChunk(dir, meta.ID, 0, strings.NewReader("data"))
	require.NoError(t, err)

	err = transfer.Cleanup(dir, meta.ID)
	require.NoError(t, err)

	_, err = os.Stat(dir + "/" + meta.ID)
	assert.True(t, os.IsNotExist(err))
}
```

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd backend && go test ./internal/transfer/... -run TestNewUpload -v
```

Expected: compilation error.

- [ ] **Step 3: Implement `upload.go`**

```go
// backend/internal/transfer/upload.go
package transfer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SessionUploadsKey is the key used in session.Data to store the uploads map.
const SessionUploadsKey = "uploads"

// UploadMeta tracks the state of a chunked upload session.
type UploadMeta struct {
	ID          string
	RemotePath  string
	TotalChunks int
	ChunkSize   int64
}

// NewUpload creates an upload session directory and returns its metadata.
// dataDir is the root directory (from config.DataDir).
func NewUpload(dataDir, remotePath string, totalChunks int, chunkSize int64) (*UploadMeta, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(b)
	if err := os.MkdirAll(filepath.Join(dataDir, id), 0o700); err != nil {
		return nil, err
	}
	return &UploadMeta{
		ID:          id,
		RemotePath:  remotePath,
		TotalChunks: totalChunks,
		ChunkSize:   chunkSize,
	}, nil
}

// WriteChunk writes chunk number n (0-indexed) to disk.
func WriteChunk(dataDir, uploadID string, n int, r io.Reader) error {
	chunkPath := filepath.Join(dataDir, uploadID, fmt.Sprintf("%04d", n))
	f, err := os.Create(chunkPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// AssembleReader returns an io.ReadCloser that reads all chunks in order.
// The caller must Close() it (which also removes temporary files).
func AssembleReader(dataDir, uploadID string, totalChunks int) (io.ReadCloser, error) {
	readers := make([]io.Reader, totalChunks)
	files := make([]*os.File, totalChunks)
	for i := 0; i < totalChunks; i++ {
		f, err := os.Open(filepath.Join(dataDir, uploadID, fmt.Sprintf("%04d", i)))
		if err != nil {
			// close already-opened files
			for j := 0; j < i; j++ {
				files[j].Close()
			}
			return nil, err
		}
		files[i] = f
		readers[i] = f
	}
	return &assembledReader{
		Reader: io.MultiReader(readers...),
		files:  files,
	}, nil
}

type assembledReader struct {
	io.Reader
	files []*os.File
}

func (a *assembledReader) Close() error {
	for _, f := range a.files {
		f.Close()
	}
	return nil
}

// Cleanup removes the upload directory and all its contents.
func Cleanup(dataDir, uploadID string) error {
	return os.RemoveAll(filepath.Join(dataDir, uploadID))
}
```

- [ ] **Step 4: Run tests**

```bash
cd backend && go test ./internal/transfer/... -v
```

Expected: all tests pass (Token tests + Upload tests).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/transfer/upload.go backend/internal/transfer/upload_test.go
git commit -m "feat: add chunked upload state management"
```

---

### Task 6: Add Go dependencies

**Files:**
- Modify: `backend/go.mod`, `backend/go.sum`

- [ ] **Step 1: Add FTP, SFTP, and promote x/crypto**

```bash
cd backend && go get github.com/jlaffaye/ftp@latest github.com/pkg/sftp@latest golang.org/x/crypto@latest
```

Expected: go.mod and go.sum updated, no errors.

- [ ] **Step 2: Verify build still passes**

```bash
cd backend && go build ./...
```

Expected: no errors (new packages are downloaded but unused warnings are fine until adapters are written).

- [ ] **Step 3: Commit**

```bash
git add backend/go.mod backend/go.sum
git commit -m "chore: add jlaffaye/ftp, pkg/sftp deps"
```

---

### Task 7: FTP adapter

**Files:**
- Create: `backend/internal/ftp/ftp.go`
- Create: `backend/internal/ftp/ftp_test.go`

- [ ] **Step 1: Write unit tests (no live server needed)**

```go
// backend/internal/ftp/ftp_test.go
package ftp_test

import (
	"os"
	"testing"

	gftp "github.com/darthsoup/goblinftp/internal/ftp"
	"github.com/stretchr/testify/assert"
)

// Integration tests require a live FTP server.
// Set GFTP_TEST_FTP_HOST=ftp.example.com:21 to run them.
func ftpHost(t *testing.T) string {
	t.Helper()
	h := os.Getenv("GFTP_TEST_FTP_HOST")
	if h == "" {
		t.Skip("set GFTP_TEST_FTP_HOST to run FTP integration tests")
	}
	return h
}

func TestDialBadHost(t *testing.T) {
	_, err := gftp.Dial("127.0.0.1:1", "user", "pass", false)
	assert.Error(t, err)
}

func TestDialIntegration(t *testing.T) {
	host := ftpHost(t)
	user := os.Getenv("GFTP_TEST_FTP_USER")
	pass := os.Getenv("GFTP_TEST_FTP_PASS")

	c, err := gftp.Dial(host, user, pass, true)
	assert.NoError(t, err)
	if err == nil {
		_ = c.Close()
	}
}
```

- [ ] **Step 2: Run to confirm unit test passes (integration skipped)**

```bash
cd backend && go test ./internal/ftp/... -v
```

Expected: `TestDialBadHost` PASS, `TestDialIntegration` SKIP.

- [ ] **Step 3: Implement the FTP adapter**

```go
// backend/internal/ftp/ftp.go
package ftp

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	jftp "github.com/jlaffaye/ftp"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// Client wraps jlaffaye/ftp and implements transfer.Client.
type Client struct {
	conn *jftp.ServerConn
}

// Dial connects and authenticates. passive controls passive/active mode.
func Dial(addr, user, pass string, passive bool) (*Client, error) {
	conn, err := jftp.Dial(addr, jftp.DialWithTimeout(10*time.Second))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", transfer.ErrConnectionFailed, err)
	}
	if err := conn.Login(user, pass); err != nil {
		conn.Quit()
		return nil, fmt.Errorf("%w: %v", transfer.ErrAuthFailed, err)
	}
	return &Client{conn: conn}, nil
}

func (c *Client) WorkingDir() (string, error) {
	return c.conn.CurrentDir()
}

func (c *Client) List(dir string) ([]transfer.FileInfo, error) {
	entries, err := c.conn.List(dir)
	if err != nil {
		return nil, err
	}
	out := make([]transfer.FileInfo, 0, len(entries))
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		out = append(out, transfer.FileInfo{
			Name:        e.Name,
			Size:        int64(e.Size),
			IsDir:       e.Type == jftp.EntryTypeFolder,
			ModTime:     e.Time.Unix(),
			Permissions: "",
		})
	}
	return out, nil
}

func (c *Client) Stat(p string) (transfer.FileInfo, error) {
	if p == "/" {
		return transfer.FileInfo{Name: "/", IsDir: true}, nil
	}
	parent := path.Dir(p)
	name := path.Base(p)
	entries, err := c.conn.List(parent)
	if err != nil {
		return transfer.FileInfo{}, err
	}
	for _, e := range entries {
		if e.Name == name {
			return transfer.FileInfo{
				Name:    e.Name,
				Size:    int64(e.Size),
				IsDir:   e.Type == jftp.EntryTypeFolder,
				ModTime: e.Time.Unix(),
			}, nil
		}
	}
	return transfer.FileInfo{}, fmt.Errorf("stat %s: not found", p)
}

func (c *Client) MakeDir(p string) error {
	return c.conn.MakeDir(p)
}

func (c *Client) Delete(p string) error {
	err := c.conn.Delete(p)
	if err != nil {
		// Try as directory
		return c.conn.RemoveDirRecur(p)
	}
	return nil
}

func (c *Client) Rename(src, dst string) error {
	return c.conn.Rename(src, dst)
}

func (c *Client) Chmod(p string, mode uint32) error {
	// FTP SITE CHMOD command
	cmd := fmt.Sprintf("CHMOD %04o %s", mode, p)
	_, err := c.conn.SendCommand(cmd)
	if err != nil {
		if strings.Contains(err.Error(), "not supported") || strings.Contains(err.Error(), "550") {
			return transfer.ErrPermissionsNotSupported
		}
		return err
	}
	return nil
}

func (c *Client) Download(p string) (io.ReadCloser, error) {
	return c.conn.Retr(p)
}

func (c *Client) Upload(p string, r io.Reader) error {
	return c.conn.Stor(p, r)
}

func (c *Client) Close() error {
	return c.conn.Quit()
}

// Ensure *Client implements transfer.Client at compile time.
var _ transfer.Client = (*Client)(nil)

// isAuthError checks if an FTP error looks like an auth failure.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "530") || strings.Contains(msg, "430")
}

// unused — kept for future use
var _ = errors.Is
```

- [ ] **Step 4: Run tests**

```bash
cd backend && go test ./internal/ftp/... -v
```

Expected: `TestDialBadHost` PASS, integration SKIP.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ftp/
git commit -m "feat: add FTP adapter implementing transfer.Client"
```

---

### Task 8: SFTP adapter

**Files:**
- Create: `backend/internal/sftp/sftp.go`
- Create: `backend/internal/sftp/sftp_test.go`

- [ ] **Step 1: Write the tests**

```go
// backend/internal/sftp/sftp_test.go
package sftp_test

import (
	"os"
	"testing"

	gsftp "github.com/darthsoup/goblinftp/internal/sftp"
	"github.com/stretchr/testify/assert"
)

func sftpHost(t *testing.T) string {
	t.Helper()
	h := os.Getenv("GFTP_TEST_SFTP_HOST")
	if h == "" {
		t.Skip("set GFTP_TEST_SFTP_HOST to run SFTP integration tests")
	}
	return h
}

func TestDialBadHost(t *testing.T) {
	_, err := gsftp.Dial("127.0.0.1:1", "user", "pass")
	assert.Error(t, err)
}

func TestDialIntegration(t *testing.T) {
	host := sftpHost(t)
	user := os.Getenv("GFTP_TEST_SFTP_USER")
	pass := os.Getenv("GFTP_TEST_SFTP_PASS")

	c, err := gsftp.Dial(host, user, pass)
	assert.NoError(t, err)
	if err == nil {
		_ = c.Close()
	}
}
```

- [ ] **Step 2: Run to confirm unit test passes (integration skipped)**

```bash
cd backend && go test ./internal/sftp/... -v
```

Expected: `TestDialBadHost` PASS, `TestDialIntegration` SKIP.

- [ ] **Step 3: Implement the SFTP adapter**

```go
// backend/internal/sftp/sftp.go
package sftp

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// Client wraps pkg/sftp and implements transfer.Client.
type Client struct {
	ssh  *ssh.Client
	sftp *sftp.Client
}

// Dial connects via SSH and opens an SFTP subsystem.
// Phase 3 uses InsecureIgnoreHostKey — Phase 4 will add key verification.
func Dial(addr, user, pass string) (*Client, error) {
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // Phase 4 will fix this
		Timeout:         10 * time.Second,
	}
	sshConn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		msg := err.Error()
		if isAuthErr(msg) {
			return nil, fmt.Errorf("%w: %v", transfer.ErrAuthFailed, err)
		}
		return nil, fmt.Errorf("%w: %v", transfer.ErrConnectionFailed, err)
	}
	sftpClient, err := sftp.NewClient(sshConn)
	if err != nil {
		sshConn.Close()
		return nil, fmt.Errorf("%w: %v", transfer.ErrConnectionFailed, err)
	}
	return &Client{ssh: sshConn, sftp: sftpClient}, nil
}

func (c *Client) WorkingDir() (string, error) {
	return c.sftp.Getwd()
}

func (c *Client) List(dir string) ([]transfer.FileInfo, error) {
	entries, err := c.sftp.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]transfer.FileInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, infoFromFS(e))
	}
	return out, nil
}

func (c *Client) Stat(p string) (transfer.FileInfo, error) {
	fi, err := c.sftp.Stat(p)
	if err != nil {
		return transfer.FileInfo{}, err
	}
	return infoFromFS(fi), nil
}

func (c *Client) MakeDir(p string) error {
	return c.sftp.MkdirAll(p)
}

func (c *Client) Delete(p string) error {
	fi, err := c.sftp.Stat(p)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return c.sftp.RemoveAll(p)
	}
	return c.sftp.Remove(p)
}

func (c *Client) Rename(src, dst string) error {
	return c.sftp.Rename(src, dst)
}

func (c *Client) Chmod(p string, mode uint32) error {
	err := c.sftp.Chmod(p, fs.FileMode(mode))
	if errors.Is(err, sftp.ErrSSHFxOpUnsupported) {
		return transfer.ErrPermissionsNotSupported
	}
	return err
}

func (c *Client) Download(p string) (io.ReadCloser, error) {
	return c.sftp.Open(p)
}

func (c *Client) Upload(p string, r io.Reader) error {
	f, err := c.sftp.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (c *Client) Close() error {
	sftpErr := c.sftp.Close()
	sshErr := c.ssh.Close()
	if sftpErr != nil {
		return sftpErr
	}
	return sshErr
}

var _ transfer.Client = (*Client)(nil)

func infoFromFS(fi fs.FileInfo) transfer.FileInfo {
	return transfer.FileInfo{
		Name:        fi.Name(),
		Size:        fi.Size(),
		IsDir:       fi.IsDir(),
		ModTime:     fi.ModTime().Unix(),
		Permissions: fi.Mode().String(),
	}
}

func isAuthErr(msg string) bool {
	return contains(msg, "unable to authenticate") ||
		contains(msg, "permission denied") ||
		contains(msg, "auth fail")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		len(s) > 0 && func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
```

Note: replace the `contains` helper with `strings.Contains`:

```go
import "strings"
// ...
func isAuthErr(msg string) bool {
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "auth fail")
}
```

And remove the `contains` function. The final file should use `strings.Contains`.

- [ ] **Step 4: Run tests**

```bash
cd backend && go test ./internal/sftp/... -v
```

Expected: `TestDialBadHost` PASS, `TestDialIntegration` SKIP.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/sftp/
git commit -m "feat: add SFTP adapter implementing transfer.Client"
```

---

### Task 9: Handler infrastructure + Connect wiring

**Files:**
- Modify: `backend/internal/api/handler.go`
- Create: `backend/internal/api/dial.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/connect.go`
- Modify: `backend/internal/api/connect_test.go`
- Modify: `backend/internal/api/router_test.go`

- [ ] **Step 1: Update `handler.go`**

Replace the current `Handler` struct with:

```go
// backend/internal/api/handler.go
package api

import (
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/session"
	"github.com/darthsoup/goblinftp/internal/throttle"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// DialFunc creates a transfer.Client for the given protocol, address, credentials, and passive flag.
type DialFunc func(protocol, addr, user, pass string, passive bool) (transfer.Client, error)

// HandlerOption is a functional option for constructing a Handler.
type HandlerOption func(*Handler)

// WithDial overrides the dial function (primarily for testing).
func WithDial(fn DialFunc) HandlerOption {
	return func(h *Handler) {
		h.dial = fn
	}
}

// Handler holds shared dependencies for all API handlers.
type Handler struct {
	cfg      *config.Config
	store    *session.Store
	throttle *throttle.Throttle
	dataDir  string
	dial     DialFunc
}

func newHandler(cfg *config.Config, store *session.Store, thr *throttle.Throttle, opts []HandlerOption) *Handler {
	h := &Handler{
		cfg:      cfg,
		store:    store,
		throttle: thr,
		dataDir:  cfg.DataDir,
		dial:     defaultDial,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}
```

- [ ] **Step 2: Create `dial.go`**

```go
// backend/internal/api/dial.go
package api

import (
	"fmt"

	ftpadapter "github.com/darthsoup/goblinftp/internal/ftp"
	sftpadapter "github.com/darthsoup/goblinftp/internal/sftp"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// defaultDial routes to the FTP or SFTP adapter based on protocol.
func defaultDial(protocol, addr, user, pass string, passive bool) (transfer.Client, error) {
	switch protocol {
	case "ftp":
		return ftpadapter.Dial(addr, user, pass, passive)
	case "sftp":
		return sftpadapter.Dial(addr, user, pass)
	default:
		return nil, fmt.Errorf("%w: unknown protocol %q", transfer.ErrConnectionFailed, protocol)
	}
}
```

- [ ] **Step 3: Update `router.go` signature to variadic**

Find the current `Register` function signature:

```go
func Register(e *echo.Echo, cfg *config.Config, store *session.Store, thr *throttle.Throttle) {
```

Replace with:

```go
func Register(e *echo.Echo, cfg *config.Config, store *session.Store, thr *throttle.Throttle, opts ...HandlerOption) {
    h := newHandler(cfg, store, thr, opts)
```

Also in `router.go`, move `GET /system/vars` out of the `requireSession` group — it should be registered directly on `e` (no middleware), and add `POST /files/download-token` in the session-protected group:

```go
// Public route (no auth required):
e.GET("/api/system/vars", h.SystemVars)

// In the requireSession group, add:
files.POST("/download-token", h.IssueDownloadToken)
```

- [ ] **Step 4: Update `connect.go` — add `Passive bool`, implement dial + session**

Replace the `ConnectRequest` struct definition:

```go
type ConnectRequest struct {
    Protocol string `json:"protocol" validate:"required,oneof=ftp sftp"`
    Host     string `json:"host"     validate:"required"`
    Port     int    `json:"port"     validate:"required,min=1,max=65535"`
    Username string `json:"username" validate:"required"`
    Password string `json:"password" validate:"required"`
    Passive  bool   `json:"passive"`
}
```

Replace the TODO dial step in `Connect()` with:

```go
addr := fmt.Sprintf("%s:%d", req.Host, req.Port)
client, err := h.dial(req.Protocol, addr, req.Username, req.Password, req.Passive)
if err != nil {
    h.throttle.Record(clientIP)
    if errors.Is(err, transfer.ErrAuthFailed) {
        return errors.Fail(c, errors.ErrAuthFailed)
    }
    return errors.Fail(c, errors.ErrConnectionFailed)
}
h.throttle.Reset(clientIP)

initialDir, err := client.WorkingDir()
if err != nil {
    client.Close()
    return errors.Fail(c, errors.ErrConnectionFailed)
}

disableChmod := detectChmod(client, req.Protocol, initialDir)

csrfToken := auth.GenerateCSRFToken()
sess, err := h.store.New(c)
if err != nil {
    client.Close()
    return errors.Fail(c, errors.ErrInternal)
}
sess.Data["client"] = client
sess.Data[auth.CSRFSessionKey] = csrfToken
sess.Data["initialDir"] = initialDir

return response.OK(c, ConnectData{
    Capabilities: Capabilities{
        DisableChmod: disableChmod,
    },
    InitialDirectory: initialDir,
    CSRFToken:        csrfToken,
})
```

Add the `detectChmod` helper at the bottom of `connect.go`:

```go
// detectChmod returns true (disable chmod) when the server doesn't support it.
func detectChmod(client transfer.Client, protocol, dir string) bool {
	if protocol == "ftp" {
		return false // FTP servers may support SITE CHMOD; assume yes
	}
	// SFTP: probe with a no-op chmod on the initial directory
	fi, err := client.Stat(dir)
	if err != nil {
		return true
	}
	err = client.Chmod(dir, uint32(fi.Permissions[0])) // same mode, no change
	return errors.Is(err, transfer.ErrPermissionsNotSupported)
}
```

Note: `fi.Permissions` is a string in our `FileInfo`. For the chmod probe, we should use a fixed safe mode. Replace with:

```go
func detectChmod(client transfer.Client, protocol, dir string) bool {
	if protocol == "ftp" {
		return false
	}
	err := client.Chmod(dir, 0o755)
	return errors.Is(err, transfer.ErrPermissionsNotSupported)
}
```

Add the necessary imports to `connect.go`:
```go
import (
    "errors"
    "fmt"
    "github.com/darthsoup/goblinftp/internal/auth"
    apierrors "github.com/darthsoup/goblinftp/internal/errors"
    "github.com/darthsoup/goblinftp/internal/transfer"
    "github.com/darthsoup/goblinftp/internal/api/response"
    ...
)
```

(Follow the exact import alias pattern already used in `connect.go`.)

Also update `Disconnect()` to close the client:

```go
func (h *Handler) Disconnect(c echo.Context) error {
    sess, err := h.store.Get(c)
    if err != nil {
        return response.Fail(c, apierrors.ErrSessionNotFound)
    }
    if client, ok := sess.Data["client"].(transfer.Client); ok {
        _ = client.Close()
    }
    _ = h.store.Delete(c)
    return response.OK(c, nil)
}
```

- [ ] **Step 5: Replace `TestConnectValidReturns501` in `connect_test.go`**

Find and remove the test `TestConnectValidReturns501` (around line 140). Replace with:

```go
func TestConnectSuccess(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/home/user", nil },
		ChmodFn:      func(path string, mode uint32) error { return nil },
	}
	dialFn := func(protocol, addr, user, pass string, passive bool) (transfer.Client, error) {
		return mock, nil
	}

	app := newTestApp(t, api.WithDial(dialFn))
	body := `{"protocol":"ftp","host":"ftp.example.com","port":21,"username":"user","password":"pass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			InitialDirectory string `json:"initialDirectory"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "/home/user", resp.Data.InitialDirectory)
}
```

Add imports at the top of `connect_test.go` (merge with existing):
```go
import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/darthsoup/goblinftp/internal/api"
    "github.com/darthsoup/goblinftp/internal/transfer"
    "github.com/darthsoup/goblinftp/internal/transfer/testutil"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)
```

- [ ] **Step 6: Update `router_test.go`**

Change `newTestApp` to accept variadic opts:

```go
func newTestApp(t *testing.T, opts ...api.HandlerOption) *echo.Echo {
    t.Helper()
    e := echo.New()
    cfg := defaultTestConfig()
    store := session.NewStore(cfg)
    thr := throttle.New()
    api.Register(e, cfg, store, thr, opts...)
    return e
}
```

Add `DataDir` to `defaultTestConfig()`:

```go
func defaultTestConfig() *config.Config {
    return &config.Config{
        // ... existing fields ...
        DataDir: os.TempDir(),
        // ... rest ...
    }
}
```

Add `"os"` to imports if not already present.

- [ ] **Step 7: Run all existing tests**

```bash
cd backend && go test ./internal/api/... -v
```

Expected: all tests pass (including the new `TestConnectSuccess`).

- [ ] **Step 8: Commit**

```bash
git add backend/internal/api/
git commit -m "feat: wire dial function, functional options, connect handler"
```

---

### Task 10: System vars handler

**Files:**
- Create: `backend/internal/api/system.go`
- Create: `backend/internal/api/system_test.go`
- Modify: `backend/internal/api/router.go` (move route to public)

- [ ] **Step 1: Write the test**

```go
// backend/internal/api/system_test.go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemVarsPublic(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/api/system/vars", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
}

func TestSystemVarsNoSession(t *testing.T) {
	// Calling /api/system/vars should work WITHOUT a session cookie
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/api/system/vars", nil)
	// No cookie set
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
```

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd backend && go test ./internal/api/... -run TestSystemVars -v
```

Expected: FAIL — handler not implemented yet.

- [ ] **Step 3: Implement `system.go`**

The response mirrors MonstaFTP's `/api/system/vars`. Omit `access.allowedClientAddresses` for security.

```go
// backend/internal/api/system.go
package api

import (
	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/api/response"
)

type systemVarsData struct {
	InitialPath     string      `json:"initialPath"`
	Language        string      `json:"language"`
	Access          accessVars  `json:"access"`
	Upload          uploadVars  `json:"upload"`
	Display         displayVars `json:"display"`
}

type accessVars struct {
	// Note: allowedClientAddresses is intentionally omitted (security)
	DisabledActions []string `json:"disabledActions"`
}

type uploadVars struct {
	MaxFileSize int64  `json:"maxFileSize"`
	ChunkSize   int64  `json:"chunkSize"`
}

type displayVars struct {
	DateFormat string `json:"dateFormat"`
}

func (h *Handler) SystemVars(c echo.Context) error {
	return response.OK(c, systemVarsData{
		InitialPath: h.cfg.InitialPath,
		Language:    h.cfg.Language,
		Access: accessVars{
			DisabledActions: h.cfg.DisabledActions,
		},
		Upload: uploadVars{
			MaxFileSize: h.cfg.MaxFileSize,
			ChunkSize:   h.cfg.ChunkSize,
		},
		Display: displayVars{
			DateFormat: h.cfg.DateFormat,
		},
	})
}
```

**NOTE:** The fields above (`InitialPath`, `Language`, `DisabledActions`, `MaxFileSize`, `DateFormat`) need to exist on `Config`. If they don't, look at what fields `Config` actually has and use the appropriate ones — or add them if the spec requires them. Check `backend/internal/config/config.go` first and adapt the struct to what actually exists.

A minimal safe version that always compiles:

```go
func (h *Handler) SystemVars(c echo.Context) error {
	return response.OK(c, map[string]interface{}{
		"chunkSize": h.cfg.ChunkSize,
	})
}
```

Start with the minimal version; expand once you know the full Config shape.

- [ ] **Step 4: Move the route in `router.go`**

In `router.go`, the current registration should be:
```go
e.GET("/api/system/vars", h.SystemVars)  // PUBLIC — no requireSession
```

Ensure it is NOT inside the `requireSession` group.

- [ ] **Step 5: Run tests**

```bash
cd backend && go test ./internal/api/... -run TestSystemVars -v
```

Expected: both tests pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/api/system.go backend/internal/api/system_test.go backend/internal/api/router.go
git commit -m "feat: implement system/vars handler (public, no session required)"
```

---

### Task 11: File operation handlers

**Files:**
- Create: `backend/internal/api/files.go`
- Create: `backend/internal/api/files_test.go`

These handlers all require a valid session with a `transfer.Client` stored under `sess.Data["client"]`. We use `MockClient` to inject behavior in tests.

- [ ] **Step 1: Write the failing tests**

```go
// backend/internal/api/files_test.go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: establish a session and return the session cookie
func connectWithMock(t *testing.T, app interface{ ServeHTTP(http.ResponseWriter, *http.Request) }, mock *testutil.MockClient, dialFn api.DialFunc) []*http.Cookie {
	t.Helper()
	// Use the TestConnectSuccess pattern to get a session cookie
	body := `{"protocol":"ftp","host":"h","port":21,"username":"u","password":"p"}`
	req := httptest.NewRequest(http.MethodPost, "/api/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	return rec.Result().Cookies()
}

func TestListFiles(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		ListFn: func(path string) ([]transfer.FileInfo, error) {
			return []transfer.FileInfo{
				{Name: "file.txt", Size: 100, IsDir: false},
				{Name: "subdir", IsDir: true},
			}, nil
		},
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app := newTestApp(t, api.WithDial(dialFn))
	cookies := connectWithMock(t, app, mock, dialFn)

	req := httptest.NewRequest(http.MethodGet, "/api/files/list?path=/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Files []transfer.FileInfo `json:"files"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Len(t, resp.Data.Files, 2)
}

func TestCreateDirectory(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		MakeDirFn:    func(path string) error { return nil },
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app := newTestApp(t, api.WithDial(dialFn))
	cookies := connectWithMock(t, app, mock, dialFn)

	body := `{"path":"/newdir"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/mkdir", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDeleteFilesPartialFailure(t *testing.T) {
	callCount := 0
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		DeleteFn: func(path string) error {
			callCount++
			if path == "/bad.txt" {
				return fmt.Errorf("permission denied")
			}
			return nil
		},
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app := newTestApp(t, api.WithDial(dialFn))
	cookies := connectWithMock(t, app, mock, dialFn)

	body := `{"paths":["/good.txt","/bad.txt"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMultiStatus, rec.Code)
}
```

Add `"fmt"` to imports.

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd backend && go test ./internal/api/... -run "TestListFiles|TestCreateDirectory|TestDeleteFiles" -v
```

Expected: compilation errors — handlers not defined.

- [ ] **Step 3: Implement `files.go`**

```go
// backend/internal/api/files.go
package api

import (
	"net/http"

	"github.com/labstack/echo/v4"

	apierrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/api/response"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

func (h *Handler) client(c echo.Context) (transfer.Client, error) {
	sess, err := h.store.Get(c)
	if err != nil {
		return nil, apierrors.ErrSessionNotFound
	}
	client, ok := sess.Data["client"].(transfer.Client)
	if !ok {
		return nil, apierrors.ErrSessionNotFound
	}
	return client, nil
}

func (h *Handler) ListFiles(c echo.Context) error {
	client, err := h.client(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	path := c.QueryParam("path")
	if path == "" {
		path = "/"
	}
	files, err := client.List(path)
	if err != nil {
		return response.Fail(c, apierrors.ErrListFailed)
	}
	return response.OK(c, map[string]interface{}{"files": files})
}

func (h *Handler) CreateDirectory(c echo.Context) error {
	client, err := h.client(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	var req struct {
		Path string `json:"path" validate:"required"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	if err := client.MakeDir(req.Path); err != nil {
		return response.Fail(c, apierrors.ErrOperationFailed)
	}
	return response.OK(c, nil)
}

type deleteResult struct {
	Deleted []string            `json:"deleted"`
	Failed  []deleteFailedEntry `json:"failed"`
}

type deleteFailedEntry struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

func (h *Handler) DeleteFiles(c echo.Context) error {
	client, err := h.client(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	var req struct {
		Paths []string `json:"paths" validate:"required,min=1"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	result := deleteResult{}
	for _, p := range req.Paths {
		if err := client.Delete(p); err != nil {
			result.Failed = append(result.Failed, deleteFailedEntry{Path: p, Error: err.Error()})
		} else {
			result.Deleted = append(result.Deleted, p)
		}
	}
	if len(result.Failed) > 0 {
		return c.JSON(http.StatusMultiStatus, response.Response{
			Success: len(result.Deleted) > 0,
			Data:    result,
		})
	}
	return response.OK(c, result)
}

func (h *Handler) RenameFile(c echo.Context) error {
	client, err := h.client(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	var req struct {
		From string `json:"from" validate:"required"`
		To   string `json:"to"   validate:"required"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	if err := client.Rename(req.From, req.To); err != nil {
		return response.Fail(c, apierrors.ErrOperationFailed)
	}
	return response.OK(c, nil)
}

func (h *Handler) CopyFile(c echo.Context) error {
	client, err := h.client(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	var req struct {
		From string `json:"from" validate:"required"`
		To   string `json:"to"   validate:"required"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	// FTP/SFTP don't have native copy — download then upload
	r, err := client.Download(req.From)
	if err != nil {
		return response.Fail(c, apierrors.ErrFileNotFound)
	}
	defer r.Close()
	if err := client.Upload(req.To, r); err != nil {
		return response.Fail(c, apierrors.ErrOperationFailed)
	}
	return response.OK(c, nil)
}

func (h *Handler) SetPermissions(c echo.Context) error {
	client, err := h.client(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	var req struct {
		Path string `json:"path" validate:"required"`
		Mode uint32 `json:"mode" validate:"required"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	if err := client.Chmod(req.Path, req.Mode); err != nil {
		if errors.Is(err, transfer.ErrPermissionsNotSupported) {
			return response.Fail(c, apierrors.ErrPermissionsNotSupported)
		}
		return response.Fail(c, apierrors.ErrOperationFailed)
	}
	return response.OK(c, nil)
}
```

Add `"errors"` to imports.

**NOTE on `response.Response`:** The `Response` struct is in `response.go`. Verify it exports `Response{Success bool, Data interface{}, Errors interface{}}` before using it directly. If it's unexported or has different fields, use `c.JSON(http.StatusMultiStatus, map[string]interface{}{...})` instead.

- [ ] **Step 4: Run tests**

```bash
cd backend && go test ./internal/api/... -run "TestListFiles|TestCreateDirectory|TestDeleteFiles" -v
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/files.go backend/internal/api/files_test.go
git commit -m "feat: implement file operation handlers (list, mkdir, delete, rename, copy, chmod)"
```

---

### Task 12: Download handlers

**Files:**
- Create: `backend/internal/api/download.go`
- Create: `backend/internal/api/download_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// backend/internal/api/download_test.go
package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueDownloadToken(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app := newTestApp(t, api.WithDial(dialFn))
	cookies := connectWithMock(t, app, mock, dialFn)

	body := `{"path":"/file.txt"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/download-token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool   `json:"success"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.Data.Token)
}

func TestDownloadFile(t *testing.T) {
	content := "hello file content"
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		DownloadFn: func(path string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(content)), nil
		},
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app := newTestApp(t, api.WithDial(dialFn))
	cookies := connectWithMock(t, app, mock, dialFn)

	// First get a token
	tokenBody := `{"path":"/file.txt"}`
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/files/download-token", strings.NewReader(tokenBody))
	tokenReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		tokenReq.AddCookie(c)
	}
	tokenRec := httptest.NewRecorder()
	app.ServeHTTP(tokenRec, tokenReq)
	require.Equal(t, http.StatusOK, tokenRec.Code)

	var tokenResp struct {
		Data struct{ Token string `json:"token"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(tokenRec.Body.Bytes(), &tokenResp))
	token := tokenResp.Data.Token

	// Use the token to download
	dlReq := httptest.NewRequest(http.MethodGet, "/api/files/download?token="+token, nil)
	for _, c := range cookies {
		dlReq.AddCookie(c)
	}
	dlRec := httptest.NewRecorder()
	app.ServeHTTP(dlRec, dlReq)

	assert.Equal(t, http.StatusOK, dlRec.Code)
	assert.Equal(t, content, dlRec.Body.String())
}
```

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd backend && go test ./internal/api/... -run "TestIssueDownloadToken|TestDownloadFile" -v
```

Expected: compilation error.

- [ ] **Step 3: Implement `download.go`**

```go
// backend/internal/api/download.go
package api

import (
	"archive/zip"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/labstack/echo/v4"

	apierrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/api/response"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

func (h *Handler) IssueDownloadToken(c echo.Context) error {
	sess, err := h.store.Get(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	var req struct {
		Path string `json:"path" validate:"required"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	expiry := time.Now().Add(15 * time.Minute)
	tok, err := transfer.IssueToken(h.cfg.DownloadTokenSecret, sess.ID, req.Path, expiry)
	if err != nil {
		return response.Fail(c, apierrors.ErrInternal)
	}
	return response.OK(c, map[string]string{"token": tok})
}

func (h *Handler) DownloadFile(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return response.Fail(c, apierrors.ErrInvalidToken)
	}
	sessionID, filePath, err := transfer.ValidateToken(h.cfg.DownloadTokenSecret, token)
	if err != nil {
		return response.Fail(c, apierrors.ErrInvalidToken)
	}
	sess, err := h.store.GetByID(c, sessionID)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	client, ok := sess.Data["client"].(transfer.Client)
	if !ok {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	r, err := client.Download(filePath)
	if err != nil {
		return response.Fail(c, apierrors.ErrFileNotFound)
	}
	defer r.Close()

	filename := path.Base(filePath)
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().WriteHeader(http.StatusOK)
	_, err = io.Copy(c.Response(), r)
	return err
}

func (h *Handler) DownloadZip(c echo.Context) error {
	sess, err := h.store.Get(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	client, ok := sess.Data["client"].(transfer.Client)
	if !ok {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	var req struct {
		Paths []string `json:"paths" validate:"required,min=1"`
		Name  string   `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	archiveName := req.Name
	if archiveName == "" {
		archiveName = "archive.zip"
	}

	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+archiveName+`"`)
	c.Response().Header().Set("Content-Type", "application/zip")
	c.Response().WriteHeader(http.StatusOK)

	zw := zip.NewWriter(c.Response())
	for _, p := range req.Paths {
		if err := addToZip(zw, client, p, ""); err != nil {
			// Can't change status after WriteHeader — just stop streaming
			break
		}
	}
	return zw.Close()
}

// addToZip recursively adds a file or directory to the zip writer.
func addToZip(zw *zip.Writer, client transfer.Client, remotePath, base string) error {
	fi, err := client.Stat(remotePath)
	if err != nil {
		return err
	}
	entryName := base + fi.Name
	if fi.IsDir {
		entryName += "/"
		entries, err := client.List(remotePath)
		if err != nil {
			return err
		}
		for _, e := range entries {
			childPath := remotePath + "/" + e.Name
			if err := addToZip(zw, client, childPath, entryName); err != nil {
				return err
			}
		}
		return nil
	}
	w, err := zw.Create(entryName)
	if err != nil {
		return err
	}
	r, err := client.Download(remotePath)
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = io.Copy(w, r)
	return err
}
```

**NOTE on `h.store.GetByID`:** Check whether `session.Store` has a `GetByID(ctx, id)` method. If it doesn't, you'll need to add it — or change `DownloadFile` to look up the session from the cookie instead (simpler, but less flexible). For Phase 3, the simplest approach is to use the session from the cookie (requiring the user to have the same session open) and ignore the `sessionID` from the token for session lookup, only using it for validation:

```go
// Simpler alternative for DownloadFile:
func (h *Handler) DownloadFile(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return response.Fail(c, apierrors.ErrInvalidToken)
	}
	// Validate token (checks HMAC and expiry)
	_, filePath, err := transfer.ValidateToken(h.cfg.DownloadTokenSecret, token)
	if err != nil {
		return response.Fail(c, apierrors.ErrInvalidToken)
	}
	// Get client from current session
	client, err := h.client(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	r, err := client.Download(filePath)
	if err != nil {
		return response.Fail(c, apierrors.ErrFileNotFound)
	}
	defer r.Close()
	filename := path.Base(filePath)
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().WriteHeader(http.StatusOK)
	_, err = io.Copy(c.Response(), r)
	return err
}
```

Use this simpler version unless `GetByID` already exists.

- [ ] **Step 4: Run tests**

```bash
cd backend && go test ./internal/api/... -run "TestIssueDownloadToken|TestDownloadFile" -v
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/download.go backend/internal/api/download_test.go
git commit -m "feat: implement download token and file/zip download handlers"
```

---

### Task 13: Upload handlers

**Files:**
- Create: `backend/internal/api/upload.go`
- Create: `backend/internal/api/upload_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// backend/internal/api/upload_test.go
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darthsoup/goblinftp/internal/api"
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
	app := newTestApp(t, api.WithDial(dialFn))
	cookies := connectWithMock(t, app, mock, dialFn)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("path", "/uploads/test.txt")
	part, _ := writer.CreateFormFile("file", "test.txt")
	_, _ = io.WriteString(part, "file contents here")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/files/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
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
	app := newTestApp(t, api.WithDial(dialFn))
	cookies := connectWithMock(t, app, mock, dialFn)

	// Reserve
	reserveBody := `{"path":"/big.bin","totalChunks":2,"totalSize":10,"chunkSize":5}`
	reserveReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/reserve", strings.NewReader(reserveBody))
	reserveReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		reserveReq.AddCookie(c)
	}
	reserveRec := httptest.NewRecorder()
	app.ServeHTTP(reserveRec, reserveReq)
	require.Equal(t, http.StatusOK, reserveRec.Code)

	var reserveResp struct {
		Data struct{ UploadID string `json:"uploadId"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(reserveRec.Body.Bytes(), &reserveResp))
	uploadID := reserveResp.Data.UploadID
	require.NotEmpty(t, uploadID)

	// Upload chunk 0
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
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}
	sendChunk(0, "hello")
	sendChunk(1, "world")

	// Commit
	commitBody := fmt.Sprintf(`{"uploadId":%q}`, uploadID)
	commitReq := httptest.NewRequest(http.MethodPost, "/api/files/upload/commit", strings.NewReader(commitBody))
	commitReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		commitReq.AddCookie(c)
	}
	commitRec := httptest.NewRecorder()
	app.ServeHTTP(commitRec, commitReq)
	require.Equal(t, http.StatusOK, commitRec.Code)
	assert.Equal(t, "helloworld", assembled)
}
```

Add `"fmt"` to imports.

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd backend && go test ./internal/api/... -run "TestUploadSimple|TestUploadChunked" -v
```

Expected: compilation error.

- [ ] **Step 3: Implement `upload.go`**

```go
// backend/internal/api/upload.go
package api

import (
	"fmt"
	"strconv"

	"github.com/labstack/echo/v4"

	apierrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/api/response"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

func (h *Handler) UploadSimple(c echo.Context) error {
	client, err := h.client(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	remotePath := c.FormValue("path")
	if remotePath == "" {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	f, err := fh.Open()
	if err != nil {
		return response.Fail(c, apierrors.ErrInternal)
	}
	defer f.Close()
	if err := client.Upload(remotePath, f); err != nil {
		return response.Fail(c, apierrors.ErrOperationFailed)
	}
	return response.OK(c, nil)
}

func (h *Handler) UploadReserve(c echo.Context) error {
	sess, err := h.store.Get(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	var req struct {
		Path        string `json:"path"        validate:"required"`
		TotalChunks int    `json:"totalChunks" validate:"required,min=1"`
		TotalSize   int64  `json:"totalSize"   validate:"required"`
		ChunkSize   int64  `json:"chunkSize"   validate:"required"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	meta, err := transfer.NewUpload(h.dataDir, req.Path, req.TotalChunks, req.ChunkSize)
	if err != nil {
		return response.Fail(c, apierrors.ErrInternal)
	}
	uploads := getUploadsMap(sess)
	uploads[meta.ID] = meta
	sess.Data[transfer.SessionUploadsKey] = uploads
	return response.OK(c, map[string]string{"uploadId": meta.ID})
}

func (h *Handler) UploadChunk(c echo.Context) error {
	sess, err := h.store.Get(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	uploadID := c.FormValue("uploadId")
	chunkIndexStr := c.FormValue("chunkIndex")
	if uploadID == "" || chunkIndexStr == "" {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	uploads := getUploadsMap(sess)
	meta, ok := uploads[uploadID]
	if !ok {
		return response.Fail(c, apierrors.ErrUploadNotFound)
	}
	fh, err := c.FormFile("chunk")
	if err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	f, err := fh.Open()
	if err != nil {
		return response.Fail(c, apierrors.ErrInternal)
	}
	defer f.Close()
	if err := transfer.WriteChunk(h.dataDir, meta.ID, chunkIndex, f); err != nil {
		return response.Fail(c, apierrors.ErrInternal)
	}
	return response.OK(c, nil)
}

func (h *Handler) UploadCommit(c echo.Context) error {
	sess, err := h.store.Get(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	client, ok := sess.Data["client"].(transfer.Client)
	if !ok {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	var req struct {
		UploadID string `json:"uploadId" validate:"required"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	uploads := getUploadsMap(sess)
	meta, ok := uploads[req.UploadID]
	if !ok {
		return response.Fail(c, apierrors.ErrUploadNotFound)
	}
	r, err := transfer.AssembleReader(h.dataDir, meta.ID, meta.TotalChunks)
	if err != nil {
		return response.Fail(c, apierrors.ErrInternal)
	}
	defer r.Close()
	if err := client.Upload(meta.RemotePath, r); err != nil {
		return response.Fail(c, apierrors.ErrOperationFailed)
	}
	_ = transfer.Cleanup(h.dataDir, meta.ID)
	delete(uploads, req.UploadID)
	sess.Data[transfer.SessionUploadsKey] = uploads
	return response.OK(c, nil)
}

func getUploadsMap(sess interface{ Data map[string]interface{} }) map[string]*transfer.UploadMeta {
	// sess is *session.Session which has a Data map
	if m, ok := sess.Data[transfer.SessionUploadsKey]; ok {
		if uploads, ok := m.(map[string]*transfer.UploadMeta); ok {
			return uploads
		}
	}
	return map[string]*transfer.UploadMeta{}
}
```

**NOTE on `getUploadsMap` signature:** `session.Session` likely has a concrete type, not an interface. Replace the parameter type with whatever `h.store.Get(c)` returns. It's probably `*session.Session`. Look at how `sess.Data` is used in `connect.go` (which you'll have just modified) and use the same type.

The function should look like:
```go
func getUploadsMap(sess *session.Session) map[string]*transfer.UploadMeta {
	if m, ok := sess.Data[transfer.SessionUploadsKey]; ok {
		if uploads, ok := m.(map[string]*transfer.UploadMeta); ok {
			return uploads
		}
	}
	return map[string]*transfer.UploadMeta{}
}
```

- [ ] **Step 4: Run tests**

```bash
cd backend && go test ./internal/api/... -run "TestUploadSimple|TestUploadChunked" -v
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/upload.go backend/internal/api/upload_test.go
git commit -m "feat: implement upload handlers (simple and chunked)"
```

---

### Task 14: Archive handlers

**Files:**
- Create: `backend/internal/api/archive.go`
- Create: `backend/internal/api/archive_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// backend/internal/api/archive_test.go
package api_test

import (
	"archive/zip"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractZipArchive(t *testing.T) {
	var uploadedFiles []string

	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		UploadFn: func(path string, r io.Reader) error {
			uploadedFiles = append(uploadedFiles, path)
			return nil
		},
		MakeDirFn: func(path string) error { return nil },
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app := newTestApp(t, api.WithDial(dialFn))
	cookies := connectWithMock(t, app, mock, dialFn)

	// Build a small zip in memory
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, _ := zw.Create("hello.txt")
	_, _ = io.WriteString(w, "hello")
	zw.Close()

	// Upload the zip via multipart
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("destination", "/extracted/")
	part, _ := writer.CreateFormFile("archive", "test.zip")
	_, _ = io.Copy(part, &zipBuf)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/files/extract", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uploadedFiles, 1)
	assert.Equal(t, "/extracted/hello.txt", uploadedFiles[0])
}

func TestCreateZipArchive(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn: func(path string) (transfer.FileInfo, error) {
			return transfer.FileInfo{Name: "file.txt", IsDir: false, Size: 5}, nil
		},
		DownloadFn: func(path string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("hello")), nil
		},
		UploadFn: func(path string, r io.Reader) error { return nil },
	}
	dialFn := func(p, a, u, pw string, passive bool) (transfer.Client, error) { return mock, nil }
	app := newTestApp(t, api.WithDial(dialFn))
	cookies := connectWithMock(t, app, mock, dialFn)

	body := `{"paths":["/file.txt"],"destination":"/archive.zip"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/compress", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
```

- [ ] **Step 2: Run to confirm FAIL**

```bash
cd backend && go test ./internal/api/... -run "TestExtractZipArchive|TestCreateZipArchive" -v
```

Expected: compilation error.

- [ ] **Step 3: Implement `archive.go`**

```go
// backend/internal/api/archive.go
package api

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/labstack/echo/v4"

	apierrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/api/response"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// ExtractArchive extracts an uploaded archive (zip/tar/tar.gz/tar.bz2) to a destination path.
func (h *Handler) ExtractArchive(c echo.Context) error {
	client, err := h.client(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	destination := c.FormValue("destination")
	if destination == "" {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	fh, err := c.FormFile("archive")
	if err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}
	f, err := fh.Open()
	if err != nil {
		return response.Fail(c, apierrors.ErrInternal)
	}
	defer f.Close()

	filename := strings.ToLower(fh.Filename)
	switch {
	case strings.HasSuffix(filename, ".zip"):
		// zip.NewReader needs io.ReaderAt and size
		data, err := io.ReadAll(f)
		if err != nil {
			return response.Fail(c, apierrors.ErrInternal)
		}
		zr, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
		if err != nil {
			return response.Fail(c, apierrors.ErrArchiveFormat)
		}
		// Use bytes.NewReader instead
		if err := extractZip(client, zr, destination); err != nil {
			return response.Fail(c, apierrors.ErrOperationFailed)
		}
	case strings.HasSuffix(filename, ".tar.gz") || strings.HasSuffix(filename, ".tgz"):
		gr, err := gzip.NewReader(f)
		if err != nil {
			return response.Fail(c, apierrors.ErrArchiveFormat)
		}
		defer gr.Close()
		if err := extractTar(client, tar.NewReader(gr), destination); err != nil {
			return response.Fail(c, apierrors.ErrOperationFailed)
		}
	case strings.HasSuffix(filename, ".tar.bz2"):
		if err := extractTar(client, tar.NewReader(bzip2.NewReader(f)), destination); err != nil {
			return response.Fail(c, apierrors.ErrOperationFailed)
		}
	case strings.HasSuffix(filename, ".tar"):
		if err := extractTar(client, tar.NewReader(f), destination); err != nil {
			return response.Fail(c, apierrors.ErrOperationFailed)
		}
	default:
		return response.Fail(c, apierrors.ErrArchiveFormat)
	}
	return response.OK(c, nil)
}

func extractZip(client transfer.Client, zr *zip.Reader, destination string) error {
	for _, entry := range zr.File {
		outPath := path.Join(destination, entry.Name)
		if entry.FileInfo().IsDir() {
			_ = client.MakeDir(outPath)
			continue
		}
		_ = client.MakeDir(path.Dir(outPath))
		rc, err := entry.Open()
		if err != nil {
			return err
		}
		err = client.Upload(outPath, rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTar(client transfer.Client, tr *tar.Reader, destination string) error {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		outPath := path.Join(destination, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = client.MakeDir(outPath)
		case tar.TypeReg:
			_ = client.MakeDir(path.Dir(outPath))
			if err := client.Upload(outPath, tr); err != nil {
				return err
			}
		}
	}
	return nil
}

// CreateZip downloads the given paths from the remote server and uploads a new zip.
func (h *Handler) CreateZip(c echo.Context) error {
	client, err := h.client(c)
	if err != nil {
		return response.Fail(c, apierrors.ErrSessionNotFound)
	}
	var req struct {
		Paths       []string `json:"paths"       validate:"required,min=1"`
		Destination string   `json:"destination" validate:"required"`
	}
	if err := c.Bind(&req); err != nil {
		return response.Fail(c, apierrors.ErrBadRequest)
	}

	pr, pw := io.Pipe()
	zw := zip.NewWriter(pw)
	errCh := make(chan error, 1)

	go func() {
		for _, p := range req.Paths {
			if err := addToZip(zw, client, p, ""); err != nil {
				pw.CloseWithError(err)
				errCh <- err
				return
			}
		}
		zw.Close()
		pw.Close()
		errCh <- nil
	}()

	if err := client.Upload(req.Destination, pr); err != nil {
		return response.Fail(c, apierrors.ErrOperationFailed)
	}
	if err := <-errCh; err != nil {
		return response.Fail(c, apierrors.ErrOperationFailed)
	}
	return response.OK(c, nil)
}
```

**NOTE on zip bytes.Reader:** The zip extraction for `.zip` files has a type issue. Use `bytes.NewReader`:

```go
import "bytes"
// ...
case strings.HasSuffix(filename, ".zip"):
    data, err := io.ReadAll(f)
    if err != nil {
        return response.Fail(c, apierrors.ErrInternal)
    }
    zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
```

- [ ] **Step 4: Run tests**

```bash
cd backend && go test ./internal/api/... -run "TestExtractZipArchive|TestCreateZipArchive" -v
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/archive.go backend/internal/api/archive_test.go
git commit -m "feat: implement archive extract and compress handlers"
```

---

### Task 15: Router wiring and final verification

**Files:**
- Modify: `backend/internal/api/router.go` (wire all new handlers)

- [ ] **Step 1: Wire all handlers in `router.go`**

Replace the `NotImplemented` placeholders with the actual handler calls. The final route registrations should look like:

```go
func Register(e *echo.Echo, cfg *config.Config, store *session.Store, thr *throttle.Throttle, opts ...HandlerOption) {
	h := newHandler(cfg, store, thr, opts)

	// Public routes
	e.GET("/api/system/vars", h.SystemVars)
	e.GET("/api/files/download", h.DownloadFile) // token-based, no session middleware

	// Session-protected routes
	protected := e.Group("/api", requireSession(store))

	protected.POST("/connect", h.Connect)
	protected.POST("/disconnect", h.Disconnect)

	files := protected.Group("/files")
	files.GET("/list", h.ListFiles)
	files.POST("/mkdir", h.CreateDirectory)
	files.POST("/delete", h.DeleteFiles)
	files.POST("/rename", h.RenameFile)
	files.POST("/copy", h.CopyFile)
	files.POST("/chmod", h.SetPermissions)
	files.POST("/download-token", h.IssueDownloadToken)
	files.POST("/download/zip", h.DownloadZip)
	files.POST("/upload", h.UploadSimple)
	files.POST("/upload/reserve", h.UploadReserve)
	files.POST("/upload/chunk", h.UploadChunk)
	files.POST("/upload/commit", h.UploadCommit)
	files.POST("/extract", h.ExtractArchive)
	files.POST("/compress", h.CreateZip)
}
```

**NOTE:** `connect` and `disconnect` don't need `requireSession` (connect creates the session; disconnect destroys it). Look at how they were registered in Phase 2 and keep the same structure. The exact URL paths should match the spec — verify against `docs/superpowers/specs/2026-05-29-phase3-ftpsftp-layer.md`.

Also check `requireSession` signature — it takes `store *session.Store` based on Phase 2 code.

- [ ] **Step 2: Run the full test suite**

```bash
cd backend && go test ./... -v 2>&1 | tail -50
```

Expected: all tests pass, no compilation errors. Integration tests (FTP/SFTP) skip due to missing env vars.

- [ ] **Step 3: Fix any compilation errors**

Common issues to watch for:
- Missing imports (add with `goimports` or manually)
- Type mismatches between `session.Session.Data` and what handlers store
- `response.Response` being unexported vs exported
- `store.Get()` vs `store.GetByID()` missing methods

Run `go build ./...` first to get a clean error list:
```bash
cd backend && go build ./...
```

- [ ] **Step 4: Run all tests again after fixes**

```bash
cd backend && go test ./... -count=1
```

Expected: `ok` for every package.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/router.go
git commit -m "feat: wire all Phase 3 handlers into router"
```

- [ ] **Step 6: Final all-packages test and summary commit**

```bash
cd backend && go test ./... -count=1
```

If all pass:

```bash
git tag phase3-complete
```

---

## Self-Review Checklist

### Spec coverage (against `docs/superpowers/specs/2026-05-29-phase3-ftpsftp-layer.md`)

| Spec requirement | Plan task |
|---|---|
| T1: New error codes | Task 1 |
| T2: transfer.Client interface | Task 2 |
| T3: Download token | Task 4 |
| T4: Chunked upload state | Task 5 |
| T5: FTP adapter | Task 7 |
| T6: SFTP adapter | Task 8 |
| T7: Connect handler wired | Task 9 |
| T8: File ops | Task 11 |
| T9: Downloads | Task 12 |
| T10: Uploads | Task 13 |
| T11: Archives | Task 14 |
| T12: System vars | Task 10 |
| T13: Go deps | Task 6 |
| T14: Router wiring | Task 15 |

All 14 spec tasks are covered. ✓

### Verified type consistency

- `transfer.FileInfo` defined in Task 2 → used in Tasks 7, 8, 11, 12
- `transfer.ErrAuthFailed` / `ErrConnectionFailed` / `ErrPermissionsNotSupported` in Task 2 → used in Tasks 7, 8, 9
- `DialFunc` / `HandlerOption` / `WithDial` defined in Task 9 → used in test helpers throughout
- `transfer.IssueToken` / `ValidateToken` defined in Task 4 → used in Task 12
- `transfer.NewUpload` / `WriteChunk` / `AssembleReader` / `Cleanup` / `SessionUploadsKey` defined in Task 5 → used in Task 13
- `MockClient` defined in Task 3 → used in Tasks 9–14
- `h.client()` helper defined in Task 11 → reused in Tasks 12, 13, 14

### Known adaptation points (not bugs, but require checking actual codebase state)

1. **`session.Session.Data` type** — assumed to be `map[string]interface{}`. Verify this matches the actual `session` package type before implementing Tasks 9, 11–14.
2. **`store.GetByID`** — may not exist. Use the simpler cookie-based session lookup in `DownloadFile` (noted in Task 12).
3. **`response.Response` export** — Task 11 uses it directly for 207 Multi-Status. If unexported, use `c.JSON()` directly.
4. **Config fields in `SystemVars`** — Task 10 shows the minimal `chunkSize`-only version as the safe fallback. Expand based on actual `Config` struct.
5. **`connect.go` imports** — the existing `connect.go` uses specific import aliases. Match them exactly when adding the new dial/session logic.

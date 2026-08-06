// backend/internal/api/upload_conflict_test.go
package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

// remoteTree builds a StatFn where dirs report as directories, files as files,
// and everything else is missing.
func remoteTree(dirs, files []string) func(string) (transfer.FileInfo, error) {
	dirSet := make(map[string]struct{}, len(dirs))
	for _, d := range dirs {
		dirSet[d] = struct{}{}
	}
	fileSet := make(map[string]struct{}, len(files))
	for _, f := range files {
		fileSet[f] = struct{}{}
	}
	return func(p string) (transfer.FileInfo, error) {
		if _, ok := dirSet[p]; ok {
			return transfer.FileInfo{Name: p, IsDir: true}, nil
		}
		if _, ok := fileSet[p]; ok {
			return transfer.FileInfo{Name: p, Size: 42}, nil
		}
		return transfer.FileInfo{}, errors.New("not found")
	}
}

func uploadSimple(app http.Handler, sess sessionCtx, dest, content, overwrite string) *httptest.ResponseRecorder {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("path", dest)
	if overwrite != "" {
		_ = writer.WriteField("overwrite", overwrite)
	}
	part, _ := writer.CreateFormFile("file", "f.txt")
	_, _ = io.WriteString(part, content)
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/files/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addSession(req, sess)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

func conflictsFrom(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var resp struct {
		Data struct {
			Conflicts []map[string]any `json:"conflicts"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Data.Conflicts
}

// TestUploadSimpleRefusesExistingFile: without consent, an occupied destination
// is a 409 and nothing is written.
func TestUploadSimpleRefusesExistingFile(t *testing.T) {
	uploaded := false
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn:       remoteTree([]string{"/uploads"}, []string{"/uploads/test.txt"}),
		UploadFn:     func(string, io.Reader) error { uploaded = true; return nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := uploadSimple(app, sess, "/uploads/test.txt", "new", "")
	assert.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "ERR_FILE_EXISTS")
	assert.False(t, uploaded, "must not write over an existing file")
}

// TestUploadSimpleOverwriteSkipsDestinationStat: explicit consent both allows the
// write and costs no extra probe - on FTP that probe is a full parent LIST.
func TestUploadSimpleOverwriteSkipsDestinationStat(t *testing.T) {
	var statted []string
	uploaded := false
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn: func(p string) (transfer.FileInfo, error) {
			statted = append(statted, p)
			if p == "/uploads" {
				return transfer.FileInfo{IsDir: true}, nil
			}
			return transfer.FileInfo{Name: p}, nil
		},
		UploadFn: func(string, io.Reader) error { uploaded = true; return nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := uploadSimple(app, sess, "/uploads/test.txt", "new", "true")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.True(t, uploaded)
	assert.NotContains(t, statted, "/uploads/test.txt", "overwrite must skip the destination probe")
}

// TestUploadSimpleFreshParentSkipsExistenceStat: a parent we just created cannot
// already hold the destination, so the probe is skipped. Guards the optimization
// that keeps folder uploads from doubling their FTP round trips.
func TestUploadSimpleFreshParentSkipsExistenceStat(t *testing.T) {
	var statted []string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn: func(p string) (transfer.FileInfo, error) {
			statted = append(statted, p)
			return transfer.FileInfo{}, errors.New("not found")
		},
		MakeDirFn: func(string) error { return nil },
		UploadFn:  func(string, io.Reader) error { return nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := uploadSimple(app, sess, "/fresh/dir/f.txt", "data", "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.NotContains(t, statted, "/fresh/dir/f.txt", "a freshly created parent needs no probe")
}

// TestUploadSimpleStatConnectionLostFails: a probe that fails because the
// connection died must not be read as "the path is free".
func TestUploadSimpleStatConnectionLostFails(t *testing.T) {
	uploaded := false
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn: func(p string) (transfer.FileInfo, error) {
			if p == "/uploads" {
				return transfer.FileInfo{IsDir: true}, nil
			}
			return transfer.FileInfo{}, io.EOF
		},
		UploadFn: func(string, io.Reader) error { uploaded = true; return nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := uploadSimple(app, sess, "/uploads/test.txt", "data", "")
	assert.Equal(t, http.StatusBadGateway, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "ERR_CONNECTION_LOST")
	assert.False(t, uploaded, "a failed probe must never fall through to a write")
}

// TestUploadReserveRefusesExistingFile: the conflict surfaces before a single
// chunk is staged.
func TestUploadReserveRefusesExistingFile(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn:       remoteTree(nil, []string{"/big.bin"}),
	}
	store := newMemChunkStore()
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)), api.WithChunkStore(store))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/reserve",
		`{"path":"/big.bin","totalChunks":2,"totalSize":10,"chunkSize":5}`)
	assert.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "ERR_FILE_EXISTS")
	assert.Empty(t, store.reserved, "nothing should be staged for a rejected reserve")
}

func TestUploadReserveOverwriteAllowed(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn:       remoteTree(nil, []string{"/big.bin"}),
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)), api.WithChunkStore(newMemChunkStore()))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/reserve",
		`{"path":"/big.bin","totalChunks":2,"totalSize":10,"chunkSize":5,"overwrite":true}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "uploadId")
}

// TestUploadCommitConflictKeepsStagedChunks is the key regression test: a
// conflict raised at commit must leave the staged bytes intact so the client can
// re-commit with consent instead of re-uploading a multi-GB file.
func TestUploadCommitConflictKeepsStagedChunks(t *testing.T) {
	var assembled string
	exists := false
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn: func(p string) (transfer.FileInfo, error) {
			if exists && p == "/big.bin" {
				return transfer.FileInfo{Name: p}, nil
			}
			return transfer.FileInfo{}, errors.New("not found")
		},
		UploadFn: func(_ string, r io.Reader) error {
			data, _ := io.ReadAll(r)
			assembled = string(data)
			return nil
		},
	}
	store := newMemChunkStore()
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)), api.WithChunkStore(store))
	sess := connectAndGetSession(t, app)

	// Reserve while the path is free, then let someone else win the race.
	uploadID := reserveAndSendChunks(t, app, sess, "/big.bin", []string{"hello", "world"})
	exists = true

	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/commit", fmt.Sprintf(`{"uploadId":%q}`, uploadID))
	require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "ERR_FILE_EXISTS")
	assert.NotContains(t, store.cleaned, uploadID, "a conflict must not discard staged chunks")

	// The same upload now commits with consent - no bytes re-sent.
	rec = doJSON(app, sess, http.MethodPost, "/api/files/upload/commit",
		fmt.Sprintf(`{"uploadId":%q,"overwrite":true}`, uploadID))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, "helloworld", assembled)
}

// TestUploadCommitDestinationOverrideSameDirOnly: "keep both" after a race can
// retarget the commit, but only inside the directory it reserved.
func TestUploadCommitDestinationOverrideSameDirOnly(t *testing.T) {
	var uploadedPath string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn: func(p string) (transfer.FileInfo, error) {
			if p == "/data" {
				return transfer.FileInfo{IsDir: true}, nil
			}
			return transfer.FileInfo{}, errors.New("not found")
		},
		UploadFn: func(p string, r io.Reader) error {
			uploadedPath = p
			_, _ = io.ReadAll(r)
			return nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)), api.WithChunkStore(newMemChunkStore()))
	sess := connectAndGetSession(t, app)

	id := reserveAndSendChunks(t, app, sess, "/data/big.bin", []string{"hello", "world"})
	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/commit",
		fmt.Sprintf(`{"uploadId":%q,"destination":"/data/big (1).bin"}`, id))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, "/data/big (1).bin", uploadedPath)

	id2 := reserveAndSendChunks(t, app, sess, "/data/other.bin", []string{"hello", "world"})
	rec = doJSON(app, sess, http.MethodPost, "/api/files/upload/commit",
		fmt.Sprintf(`{"uploadId":%q,"destination":"/elsewhere/x.bin"}`, id2))
	assert.Equal(t, http.StatusBadRequest, rec.Code, "an uploadId must not be retargetable")
}

func TestUploadAbortCleansUpChunks(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		StatFn:       func(string) (transfer.FileInfo, error) { return transfer.FileInfo{}, errors.New("not found") },
	}
	store := newMemChunkStore()
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)), api.WithChunkStore(store))
	sess := connectAndGetSession(t, app)

	id := reserveAndSendChunks(t, app, sess, "/big.bin", []string{"hello", "world"})
	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/abort", fmt.Sprintf(`{"uploadId":%q}`, id))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, store.cleaned, id)

	rec = doJSON(app, sess, http.MethodPost, "/api/files/upload/abort", fmt.Sprintf(`{"uploadId":%q}`, id))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestUploadCheckGroupsListsByDirectory: the pre-flight costs one LIST per
// distinct directory, not one per file - FTP has no stat, so the difference is
// 2 round trips instead of 500 on a large folder upload.
func TestUploadCheckGroupsListsByDirectory(t *testing.T) {
	var listed []string
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		ListFn: func(dir string) ([]transfer.FileInfo, error) {
			listed = append(listed, dir)
			switch dir {
			case "/a":
				return []transfer.FileInfo{{Name: "x.txt", Size: 10}}, nil
			case "/b":
				return []transfer.FileInfo{{Name: "other.txt"}}, nil
			}
			return nil, nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/check",
		`{"paths":["/a/x.txt","/a/y.txt","/b/z.txt"]}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, []string{"/a", "/b"}, listed, "one LIST per distinct directory, in first-seen order")

	conflicts := conflictsFrom(t, rec)
	require.Len(t, conflicts, 1)
	assert.Equal(t, "/a/x.txt", conflicts[0]["path"])
	assert.Equal(t, "x (1).txt", conflicts[0]["suggestedName"])
}

// TestUploadCheckMissingParentIsNoConflict: an unlistable directory means the
// upload will create it, so nothing in it can conflict.
func TestUploadCheckMissingParentIsNoConflict(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		ListFn:       func(string) ([]transfer.FileInfo, error) { return nil, errors.New("550 no such directory") },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/check", `{"paths":["/nope/a.txt"]}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Empty(t, conflictsFrom(t, rec))
	assert.Contains(t, rec.Body.String(), `"conflicts":[]`, "must serialize as [] not null")
}

func TestUploadCheckSuggestsNonCollidingName(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		ListFn: func(string) ([]transfer.FileInfo, error) {
			return []transfer.FileInfo{{Name: "a.txt"}, {Name: "a (1).txt"}}, nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/check", `{"paths":["/d/a.txt"]}`)
	require.Equal(t, http.StatusOK, rec.Code)
	conflicts := conflictsFrom(t, rec)
	require.Len(t, conflicts, 1)
	assert.Equal(t, "a (2).txt", conflicts[0]["suggestedName"])
}

// TestUploadCheckSuggestionAvoidsOtherRequestedPaths: a suggestion must not land
// on a name another file in the same batch is about to occupy.
func TestUploadCheckSuggestionAvoidsOtherRequestedPaths(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		ListFn: func(string) ([]transfer.FileInfo, error) {
			return []transfer.FileInfo{{Name: "a.txt"}}, nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/check", `{"paths":["/d/a.txt","/d/a (1).txt"]}`)
	require.Equal(t, http.StatusOK, rec.Code)
	conflicts := conflictsFrom(t, rec)
	require.Len(t, conflicts, 1, "only a.txt exists remotely")
	assert.Equal(t, "a (2).txt", conflicts[0]["suggestedName"])
}

func TestUploadCheckReportsDirectoryConflict(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		ListFn: func(string) ([]transfer.FileInfo, error) {
			return []transfer.FileInfo{{Name: "photos", IsDir: true}}, nil
		},
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/check", `{"paths":["/d/photos"]}`)
	require.Equal(t, http.StatusOK, rec.Code)
	conflicts := conflictsFrom(t, rec)
	require.Len(t, conflicts, 1)
	assert.Equal(t, true, conflicts[0]["isDir"])
}

func TestUploadCheckConnectionLostAbortsBatch(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		ListFn:       func(string) ([]transfer.FileInfo, error) { return nil, io.EOF },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/check", `{"paths":["/d/a.txt"]}`)
	assert.Equal(t, http.StatusBadGateway, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "ERR_CONNECTION_LOST")
}

func TestUploadCheckBadRequest(t *testing.T) {
	mock := &testutil.MockClient{
		WorkingDirFn: func() (string, error) { return "/", nil },
		ChmodFn:      func(string, uint32) error { return nil },
		ListFn:       func(string) ([]transfer.FileInfo, error) { return nil, nil },
	}
	app, _, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	sess := connectAndGetSession(t, app)

	for name, body := range map[string]string{
		"empty":    `{"paths":[]}`,
		"relative": `{"paths":["relative/a.txt"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/check", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
		})
	}

	t.Run("too many", func(t *testing.T) {
		paths := make([]string, 10001)
		for i := range paths {
			paths[i] = fmt.Sprintf(`"/d/f%d.txt"`, i)
		}
		body := `{"paths":[` + strings.Join(paths, ",") + `]}`
		rec := doJSON(app, sess, http.MethodPost, "/api/files/upload/check", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

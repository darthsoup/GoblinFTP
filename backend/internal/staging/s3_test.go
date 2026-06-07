// backend/internal/staging/s3_test.go
//
// White-box tests: S3Store logic against an in-memory fake of the s3API
// interface — no network, no MinIO. Real-server integration tests live in
// s3_integration_test.go (gated by GFTP_TEST_S3_ENDPOINT).
package staging

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// fakeS3 is a map-backed s3API. pageSize simulates ListObjectsV2 pagination;
// err, when set, makes every call fail with a connection-level error.
type fakeS3 struct {
	mu       sync.Mutex
	objects  map[string][]byte
	pageSize int
	err      error

	openBodies int // currently open GetObject bodies
	maxOpen    int // high-water mark, to assert lazy sequential streaming
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: make(map[string][]byte)}
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	data, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.objects[*in.Key] = data
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) HeadObject(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if _, ok := f.objects[*in.Key]; !ok {
		return nil, &types.NotFound{}
	}
	return &s3.HeadObjectOutput{}, nil
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.objects[*in.Key]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	f.openBodies++
	if f.openBodies > f.maxOpen {
		f.maxOpen = f.openBodies
	}
	return &s3.GetObjectOutput{Body: &fakeBody{Reader: bytes.NewReader(data), fake: f}}, nil
}

func (f *fakeS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	var keys []string
	for k := range f.objects {
		if strings.HasPrefix(k, *in.Prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if in.ContinuationToken != nil {
		i := sort.SearchStrings(keys, *in.ContinuationToken)
		keys = keys[i:]
	}
	truncated := false
	var next *string
	if f.pageSize > 0 && len(keys) > f.pageSize {
		next = aws.String(keys[f.pageSize])
		keys = keys[:f.pageSize]
		truncated = true
	}
	out := &s3.ListObjectsV2Output{IsTruncated: aws.Bool(truncated), NextContinuationToken: next}
	for _, k := range keys {
		out.Contents = append(out.Contents, types.Object{Key: aws.String(k)})
	}
	return out, nil
}

func (f *fakeS3) DeleteObjects(_ context.Context, in *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	for _, id := range in.Delete.Objects {
		delete(f.objects, *id.Key)
	}
	return &s3.DeleteObjectsOutput{}, nil
}

func (f *fakeS3) HeadBucket(_ context.Context, _ *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return &s3.HeadBucketOutput{}, nil
}

type fakeBody struct {
	*bytes.Reader
	fake   *fakeS3
	closed bool
}

func (b *fakeBody) Close() error {
	if !b.closed {
		b.closed = true
		b.fake.mu.Lock()
		b.fake.openBodies--
		b.fake.mu.Unlock()
	}
	return nil
}

func newTestS3Store(f *fakeS3) *S3Store {
	return &S3Store{api: f, bucket: "test-bucket", prefix: "gftp-uploads", timeout: 5 * time.Second}
}

func TestS3NewUpload(t *testing.T) {
	store := newTestS3Store(newFakeS3())
	meta, err := store.NewUpload(t.Context(), "/remote/file.txt", 3, 1024)
	require.NoError(t, err)
	assert.NoError(t, transfer.ValidateUploadID(meta.ID))
	assert.Equal(t, "/remote/file.txt", meta.Destination)
	assert.Equal(t, 3, meta.TotalChunks)
	assert.Equal(t, int64(1024), meta.ChunkSize)
}

func TestS3WriteChunk(t *testing.T) {
	fake := newFakeS3()
	store := newTestS3Store(fake)
	meta, err := store.NewUpload(t.Context(), "/f.txt", 2, 5)
	require.NoError(t, err)

	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("hello")))
	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 1, 5, strings.NewReader("world")))

	assert.Equal(t, []byte("hello"), fake.objects["gftp-uploads/"+meta.ID+"/0000"])
	assert.Equal(t, []byte("world"), fake.objects["gftp-uploads/"+meta.ID+"/0001"])
}

func TestS3WriteChunk_RetryOverwrites(t *testing.T) {
	fake := newFakeS3()
	store := newTestS3Store(fake)
	meta, err := store.NewUpload(t.Context(), "/f.txt", 1, 5)
	require.NoError(t, err)

	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("first")))
	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("again")))
	assert.Equal(t, []byte("again"), fake.objects["gftp-uploads/"+meta.ID+"/0000"])
}

func TestS3WriteChunk_InvalidID(t *testing.T) {
	store := newTestS3Store(newFakeS3())
	err := store.WriteChunk(t.Context(), "not-a-uuid", 0, 1, strings.NewReader("x"))
	assert.Error(t, err)
}

func TestS3AssembleReader(t *testing.T) {
	fake := newFakeS3()
	store := newTestS3Store(fake)
	meta, err := store.NewUpload(t.Context(), "/f.txt", 3, 5)
	require.NoError(t, err)

	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("hello")))
	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 1, 5, strings.NewReader("world")))
	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 2, 1, strings.NewReader("!")))

	rc, err := store.AssembleReader(t.Context(), meta.ID, 3)
	require.NoError(t, err)

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "helloworld!", string(data))
	require.NoError(t, rc.Close())

	// Lazy sequential streaming: never more than one chunk body open at once.
	assert.LessOrEqual(t, fake.maxOpen, 1)
	assert.Equal(t, 0, fake.openBodies, "all chunk bodies must be closed")
}

func TestS3AssembleReader_MissingChunk(t *testing.T) {
	store := newTestS3Store(newFakeS3())
	meta, err := store.NewUpload(t.Context(), "/f.txt", 2, 5)
	require.NoError(t, err)

	// Write only chunk 0, not chunk 1.
	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("hello")))

	_, err = store.AssembleReader(t.Context(), meta.ID, 2)
	assert.Error(t, err)
	// The service responded (NotFound) — this is not an outage.
	assert.False(t, errors.Is(err, ErrUnavailable))
}

func TestS3AssembleReader_InvalidID(t *testing.T) {
	store := newTestS3Store(newFakeS3())
	_, err := store.AssembleReader(t.Context(), "not-a-uuid", 1)
	assert.Error(t, err)
}

func TestS3AssembleReader_CloseMidStream(t *testing.T) {
	fake := newFakeS3()
	store := newTestS3Store(fake)
	meta, err := store.NewUpload(t.Context(), "/f.txt", 2, 5)
	require.NoError(t, err)

	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("hello")))
	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 1, 5, strings.NewReader("world")))

	rc, err := store.AssembleReader(t.Context(), meta.ID, 2)
	require.NoError(t, err)

	// Read part of the first chunk, then abort (e.g. FTP sink failed).
	buf := make([]byte, 3)
	_, err = rc.Read(buf)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, 0, fake.openBodies, "open body must be closed on abort")

	// Reads after Close return EOF.
	n, err := rc.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

func TestS3Cleanup(t *testing.T) {
	fake := newFakeS3()
	store := newTestS3Store(fake)
	meta, err := store.NewUpload(t.Context(), "/f.txt", 2, 5)
	require.NoError(t, err)
	other, err := store.NewUpload(t.Context(), "/g.txt", 1, 5)
	require.NoError(t, err)

	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("hello")))
	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 1, 5, strings.NewReader("world")))
	require.NoError(t, store.WriteChunk(t.Context(), other.ID, 0, 5, strings.NewReader("other")))

	require.NoError(t, store.Cleanup(t.Context(), meta.ID))

	assert.NotContains(t, fake.objects, "gftp-uploads/"+meta.ID+"/0000")
	assert.NotContains(t, fake.objects, "gftp-uploads/"+meta.ID+"/0001")
	// Other uploads are untouched.
	assert.Contains(t, fake.objects, "gftp-uploads/"+other.ID+"/0000")
}

func TestS3Cleanup_Paginated(t *testing.T) {
	fake := newFakeS3()
	fake.pageSize = 2
	store := newTestS3Store(fake)
	meta, err := store.NewUpload(t.Context(), "/f.txt", 5, 1)
	require.NoError(t, err)

	for i := range 5 {
		require.NoError(t, store.WriteChunk(t.Context(), meta.ID, i, 1, strings.NewReader("x")))
	}

	require.NoError(t, store.Cleanup(t.Context(), meta.ID))
	assert.Empty(t, fake.objects)
}

func TestS3Cleanup_InvalidID(t *testing.T) {
	store := newTestS3Store(newFakeS3())
	err := store.Cleanup(t.Context(), "not-a-uuid")
	assert.Error(t, err)
}

func TestS3ConnectionErrorTaggedUnavailable(t *testing.T) {
	fake := newFakeS3()
	fake.err = errors.New("dial tcp 127.0.0.1:9000: connect: connection refused")
	store := newTestS3Store(fake)
	meta, err := store.NewUpload(t.Context(), "/f.txt", 1, 5)
	require.NoError(t, err)

	err = store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("hello"))
	assert.ErrorIs(t, err, ErrUnavailable)

	_, err = store.AssembleReader(t.Context(), meta.ID, 1)
	assert.ErrorIs(t, err, ErrUnavailable)

	err = store.Cleanup(t.Context(), meta.ID)
	assert.ErrorIs(t, err, ErrUnavailable)
}

func TestS3EmptyPrefix(t *testing.T) {
	fake := newFakeS3()
	store := &S3Store{api: fake, bucket: "test-bucket", prefix: "", timeout: 5 * time.Second}
	meta, err := store.NewUpload(t.Context(), "/f.txt", 1, 5)
	require.NoError(t, err)

	require.NoError(t, store.WriteChunk(t.Context(), meta.ID, 0, 5, strings.NewReader("hello")))
	assert.Contains(t, fake.objects, meta.ID+"/0000")
}

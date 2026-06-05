// backend/internal/staging/s3_integration_test.go
//
// Integration tests against a real S3-compatible server. Start one with
// `just s3-up` (MinIO on localhost:9000), then run:
//
//	GFTP_TEST_S3_ENDPOINT=http://localhost:9000 go test ./internal/staging/...
package staging_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/staging"
)

func newIntegrationStore(t *testing.T) *staging.S3Store {
	t.Helper()
	endpoint := os.Getenv("GFTP_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set GFTP_TEST_S3_ENDPOINT to run S3 integration tests (just s3-up; e.g. http://localhost:9000)")
	}
	envOr := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	store := staging.NewS3Store(staging.S3Options{
		Endpoint:     endpoint,
		Bucket:       envOr("GFTP_TEST_S3_BUCKET", "gftp-chunks"),
		Region:       envOr("GFTP_TEST_S3_REGION", "us-east-1"),
		AccessKey:    envOr("GFTP_TEST_S3_ACCESS_KEY", "minioadmin"),
		SecretKey:    envOr("GFTP_TEST_S3_SECRET_KEY", "minioadmin"),
		UsePathStyle: true,
		Prefix:       "gftp-integration-test",
		Timeout:      30 * time.Second,
	})
	require.NoError(t, store.Ping(t.Context()), "S3 server not reachable — did you run `just s3-up`?")
	return store
}

func TestS3Integration_RoundTrip(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := t.Context()

	meta, err := store.NewUpload(ctx, "/remote/file.txt", 3, 4)
	require.NoError(t, err)
	// t.Context() is canceled before cleanups run — use a fresh context.
	t.Cleanup(func() { _ = store.Cleanup(context.Background(), meta.ID) })

	require.NoError(t, store.WriteChunk(ctx, meta.ID, 0, 4, strings.NewReader("abcd")))
	require.NoError(t, store.WriteChunk(ctx, meta.ID, 1, 4, strings.NewReader("efgh")))
	require.NoError(t, store.WriteChunk(ctx, meta.ID, 2, 2, strings.NewReader("ij")))

	rc, err := store.AssembleReader(ctx, meta.ID, 3)
	require.NoError(t, err)
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, "abcdefghij", string(data))

	require.NoError(t, store.Cleanup(ctx, meta.ID))

	// After cleanup the chunks are gone.
	_, err = store.AssembleReader(ctx, meta.ID, 3)
	assert.Error(t, err)
}

func TestS3Integration_MissingChunk(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := t.Context()

	meta, err := store.NewUpload(ctx, "/remote/file.txt", 2, 4)
	require.NoError(t, err)
	// t.Context() is canceled before cleanups run — use a fresh context.
	t.Cleanup(func() { _ = store.Cleanup(context.Background(), meta.ID) })

	require.NoError(t, store.WriteChunk(ctx, meta.ID, 0, 4, strings.NewReader("abcd")))

	_, err = store.AssembleReader(ctx, meta.ID, 2)
	assert.Error(t, err)
}

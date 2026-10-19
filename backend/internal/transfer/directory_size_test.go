package transfer_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/transfer"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func directoryReader(tree map[string][]transfer.FileInfo) *testutil.MockClient {
	return &testutil.MockClient{
		LstatFn: func(string) (transfer.FileInfo, error) { return transfer.FileInfo{IsDir: true}, nil },
		ListFn: func(p string) ([]transfer.FileInfo, error) {
			entries, ok := tree[p]
			if !ok {
				return nil, transfer.ErrPermissionDenied
			}
			return entries, nil
		},
	}
}

func TestDirectorySizeRecursive(t *testing.T) {
	reader := directoryReader(map[string][]transfer.FileInfo{
		"/data": {
			{Name: "hello.txt", Size: 8},
			{Name: ".hidden", Size: 4},
			{Name: "nested", IsDir: true},
			{Name: "empty", IsDir: true},
			{Name: "linked-file", Size: 999, IsSymlink: true},
			{Name: "linked-dir", IsDir: true, IsSymlink: true},
		},
		"/data/nested": {{Name: "child.txt", Size: 12}},
		"/data/empty":  {},
	})
	result, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data/")
	require.NoError(t, err)
	assert.Equal(t, "/data", result.Path)
	assert.Equal(t, int64(24), result.Bytes)
	assert.Equal(t, 3, result.FileCount)
	assert.Equal(t, 2, result.DirectoryCount)
	assert.True(t, result.Complete)
	assert.Zero(t, result.SkippedCount)
	assert.Empty(t, result.Reason)
	_, err = time.Parse(time.RFC3339, result.CalculatedAt)
	require.NoError(t, err)
}

func TestDirectorySizeEmptyRoot(t *testing.T) {
	result, err := transfer.CalculateDirectorySize(context.Background(), directoryReader(map[string][]transfer.FileInfo{"/": {}}), "/")
	require.NoError(t, err)
	assert.True(t, result.Complete)
	assert.Zero(t, result.Bytes)
	assert.Zero(t, result.FileCount)
	assert.Zero(t, result.DirectoryCount)
	assert.False(t, result.HasEntries)
}

func TestDirectorySizeExcludedLinksAreNotEmpty(t *testing.T) {
	reader := directoryReader(map[string][]transfer.FileInfo{"/data": {{Name: "link", IsSymlink: true, Size: 999}}})
	result, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data")
	require.NoError(t, err)
	assert.True(t, result.HasEntries)
	assert.True(t, result.Complete)
	assert.Zero(t, result.Bytes)
	assert.Zero(t, result.FileCount)
	assert.Zero(t, result.DirectoryCount)
}

func TestDirectorySizeReservesReadBudgetBeforeDescending(t *testing.T) {
	reader := directoryReader(map[string][]transfer.FileInfo{
		"/data":       {{Name: "child", IsDir: true}, {Name: "root-file", Size: 2}},
		"/data/child": {{Name: "one", Size: 1}, {Name: "two", Size: 5}},
	})
	limits := make(map[string]int)
	reader.ReadDirContextFn = func(ctx context.Context, p string, limit int) ([]transfer.FileInfo, bool, error) {
		limits[p] = limit
		entries, err := reader.ListContext(ctx, p)
		if len(entries) > limit {
			return entries[:limit], true, err
		}
		return entries, false, err
	}
	result, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data", transfer.DirectorySizeOptions{MaxEntries: 3})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"/data": 3, "/data/child": 1}, limits)
	assert.Equal(t, int64(3), result.Bytes)
	assert.Equal(t, 2, result.FileCount)
	assert.False(t, result.Complete)
	assert.Equal(t, "limit", result.Reason)
}

func TestDirectorySizeDoesNotReadBeyondBudget(t *testing.T) {
	reader := directoryReader(map[string][]transfer.FileInfo{"/data": {{Name: "child", IsDir: true}, {Name: "file", Size: 3}}})
	var reads []string
	reader.ReadDirContextFn = func(ctx context.Context, p string, limit int) ([]transfer.FileInfo, bool, error) {
		reads = append(reads, p)
		entries, err := reader.ListContext(ctx, p)
		return entries, false, err
	}
	result, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data", transfer.DirectorySizeOptions{MaxEntries: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"/data"}, reads)
	assert.Equal(t, int64(3), result.Bytes)
	assert.False(t, result.Complete)
	assert.Equal(t, "limit", result.Reason)
}

func TestDirectorySizeMetadataInspectionLimitIsPartial(t *testing.T) {
	reader := directoryReader(nil)
	reader.LstatContextFn = func(context.Context, string) (transfer.FileInfo, error) {
		return transfer.FileInfo{}, transfer.ErrDirectorySizeLimit
	}
	result, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data")
	require.NoError(t, err)
	assert.False(t, result.Complete)
	assert.Equal(t, "limit", result.Reason)
}

func TestDirectorySizeRejectsUnsafeRoots(t *testing.T) {
	for _, root := range []string{"", "relative", "/a/../b", "/a/./b", "/a\\..\\b", "/bad\x00name", "/bad\r\nLIST /"} {
		t.Run(root, func(t *testing.T) {
			_, err := transfer.CalculateDirectorySize(context.Background(), &testutil.MockClient{}, root)
			require.ErrorIs(t, err, transfer.ErrInvalidType)
		})
	}
	for _, info := range []transfer.FileInfo{{}, {IsDir: true, IsSymlink: true}} {
		reader := &testutil.MockClient{LstatFn: func(string) (transfer.FileInfo, error) { return info, nil }}
		_, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data")
		require.ErrorIs(t, err, transfer.ErrInvalidType)
	}
}

func TestDirectorySizeRejectsUnsafeChildren(t *testing.T) {
	reader := directoryReader(map[string][]transfer.FileInfo{
		"/data": {
			{Name: "../escape", IsDir: true},
			{Name: "/escape", IsDir: true},
			{Name: "nested/file", Size: 99},
			{Name: "..\\escape", IsDir: true},
			{Name: "bad\nname", Size: 99},
			{Name: "", Size: 99},
			{Name: ".", IsDir: true},
			{Name: "..", IsDir: true},
			{Name: "safe", Size: 3},
			{Name: "safe", Size: 3},
		},
	})
	result, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data")
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Bytes)
	assert.Equal(t, 1, result.FileCount)
	assert.Zero(t, result.DirectoryCount)
	assert.Equal(t, 7, result.SkippedCount)
	assert.False(t, result.Complete)
	assert.Equal(t, "unreadable", result.Reason)
}

func TestDirectorySizeUnreadableDescendantIsPartial(t *testing.T) {
	reader := directoryReader(map[string][]transfer.FileInfo{"/data": {{Name: "denied", IsDir: true}, {Name: "good", Size: 6}}})
	result, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data")
	require.NoError(t, err)
	assert.Equal(t, int64(6), result.Bytes)
	assert.Equal(t, 1, result.DirectoryCount)
	assert.Equal(t, 1, result.SkippedCount)
	assert.Equal(t, "unreadable", result.Reason)
	assert.False(t, result.Complete)

	_, err = transfer.CalculateDirectorySize(context.Background(), reader, "/missing")
	require.ErrorIs(t, err, transfer.ErrPermissionDenied)
}

func TestDirectorySizeLostConnectionAborts(t *testing.T) {
	for _, lost := range []error{transfer.ErrConnectionLost, transfer.ErrConnectionTimeout, transfer.ErrDirectorySizeUnavailable} {
		reader := directoryReader(map[string][]transfer.FileInfo{"/data": {{Name: "child", IsDir: true}}})
		reader.ListContextFn = func(ctx context.Context, p string) ([]transfer.FileInfo, error) {
			if p == "/data/child" {
				return nil, lost
			}
			return reader.List(p)
		}
		_, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data")
		require.ErrorIs(t, err, lost)
	}
}

func TestDirectorySizeEntryLimit(t *testing.T) {
	reader := directoryReader(map[string][]transfer.FileInfo{"/data": {{Name: "a", Size: 1}, {Name: "b", Size: 2}, {Name: "c", Size: 3}}})
	result, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data", transfer.DirectorySizeOptions{MaxEntries: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Bytes)
	assert.Equal(t, 2, result.FileCount)
	assert.Equal(t, 1, result.SkippedCount)
	assert.Equal(t, "limit", result.Reason)
	assert.False(t, result.Complete)
}

func TestDirectorySizeDepthLimit(t *testing.T) {
	reader := directoryReader(map[string][]transfer.FileInfo{
		"/data":       {{Name: "child", IsDir: true}, {Name: "one", Size: 1}},
		"/data/child": {{Name: "deep", IsDir: true}, {Name: "two", Size: 2}},
	})
	result, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data", transfer.DirectorySizeOptions{MaxDepth: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Bytes)
	assert.Equal(t, 2, result.DirectoryCount)
	assert.Equal(t, 1, result.SkippedCount)
	assert.Equal(t, "limit", result.Reason)
}

func TestDirectorySizeInvalidAndOverflowingSizes(t *testing.T) {
	reader := directoryReader(map[string][]transfer.FileInfo{"/data": {
		{Name: "negative", Size: -1},
		{Name: "large", Size: math.MaxInt64 - 1},
		{Name: "overflow", Size: 2},
		{Name: "last", Size: 1},
	}})
	result, err := transfer.CalculateDirectorySize(context.Background(), reader, "/data")
	require.NoError(t, err)
	assert.Equal(t, int64(math.MaxInt64), result.Bytes)
	assert.Equal(t, 2, result.FileCount)
	assert.Equal(t, 2, result.SkippedCount)
	assert.Equal(t, "limit", result.Reason)
	assert.False(t, result.Complete)
}

func TestDirectorySizeTimeoutAndCancellation(t *testing.T) {
	for _, callerCancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "budget", true: "caller"}[callerCancel], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := directoryReader(map[string][]transfer.FileInfo{"/data": {{Name: "good", Size: 7}, {Name: "slow", IsDir: true}}})
			reader.ListContextFn = func(ctx context.Context, p string) ([]transfer.FileInfo, error) {
				if p == "/data/slow" {
					if callerCancel {
						cancel()
					}
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return reader.List(p)
			}
			result, err := transfer.CalculateDirectorySize(ctx, reader, "/data", transfer.DirectorySizeOptions{Timeout: 20 * time.Millisecond})
			if callerCancel {
				require.ErrorIs(t, err, context.Canceled)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, int64(7), result.Bytes)
			assert.Equal(t, "timeout", result.Reason)
			assert.False(t, result.Complete)
			assert.Positive(t, result.SkippedCount)
		})
	}
}

func TestDirectorySizePreCanceledDoesNoRemoteWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := transfer.CalculateDirectorySize(ctx, &testutil.MockClient{}, "/data")
	assert.True(t, errors.Is(err, context.Canceled))
}

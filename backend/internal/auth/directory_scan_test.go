package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/auth"
)

func TestDirectoryScanGuardIsPerSession(t *testing.T) {
	store := auth.NewStore(time.Hour)
	t.Cleanup(store.Close)
	first, err := store.New()
	require.NoError(t, err)
	second, err := store.New()
	require.NoError(t, err)
	assert.True(t, first.BeginDirectoryScan())
	assert.False(t, first.BeginDirectoryScan())
	assert.True(t, second.BeginDirectoryScan())
	first.EndDirectoryScan()
	assert.True(t, first.BeginDirectoryScan())
	first.EndDirectoryScan()
	second.EndDirectoryScan()
}

func TestLockTransferContextCancelsWhileBusy(t *testing.T) {
	store := auth.NewStore(time.Hour)
	t.Cleanup(store.Close)
	sess, err := store.New()
	require.NoError(t, err)
	sess.LockTransfer()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = sess.LockTransferContext(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, sess.TryLockTransfer())
	sess.UnlockTransfer()
	require.NoError(t, sess.LockTransferContext(context.Background()))
	sess.UnlockTransfer()
}

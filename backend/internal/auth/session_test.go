package auth_test

import (
	"sync"
	"testing"
	"time"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSession(t *testing.T) {
	store := auth.NewStore(10 * time.Minute)
	defer store.Close()

	sess, err := store.New()
	require.NoError(t, err)
	assert.NotEmpty(t, sess.ID)
	assert.NotNil(t, sess.Data)
	assert.True(t, sess.ExpiresAt.After(time.Now()))
}

func TestGetSession(t *testing.T) {
	store := auth.NewStore(10 * time.Minute)
	defer store.Close()

	sess, err := store.New()
	require.NoError(t, err)

	got, ok := store.Get(sess.ID)
	assert.True(t, ok)
	assert.Equal(t, sess.ID, got.ID)
}

func TestGetNonexistentSession(t *testing.T) {
	store := auth.NewStore(10 * time.Minute)
	defer store.Close()

	_, ok := store.Get("nonexistent-id")
	assert.False(t, ok)
}

func TestDeleteSession(t *testing.T) {
	store := auth.NewStore(10 * time.Minute)
	defer store.Close()

	sess, err := store.New()
	require.NoError(t, err)

	store.Delete(sess.ID)
	_, ok := store.Get(sess.ID)
	assert.False(t, ok)
}

func TestExpiredSessionNotReturned(t *testing.T) {
	store := auth.NewStore(50 * time.Millisecond)
	defer store.Close()

	sess, err := store.New()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	_, ok := store.Get(sess.ID)
	assert.False(t, ok, "expired session should not be returned")
}

func TestTouchExtendsExpiry(t *testing.T) {
	store := auth.NewStore(150 * time.Millisecond)
	defer store.Close()

	sess, err := store.New()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	store.Touch(sess.ID)
	time.Sleep(100 * time.Millisecond)

	_, ok := store.Get(sess.ID)
	assert.True(t, ok, "session should still be valid after touch")
}

func TestTouchDoesNotReviveExpiredSession(t *testing.T) {
	store := auth.NewStore(50 * time.Millisecond)
	defer store.Close()

	sess, err := store.New()
	require.NoError(t, err)

	time.Sleep(75 * time.Millisecond)
	store.Touch(sess.ID)

	_, ok := store.Get(sess.ID)
	assert.False(t, ok, "touch should not revive an expired session")
}

func TestGetAndTouchConcurrentNoRace(t *testing.T) {
	store := auth.NewStore(time.Second)
	defer store.Close()

	sess, err := store.New()
	require.NoError(t, err)

	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 1000; j++ {
				store.Get(sess.ID)
			}
		}()

		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 1000; j++ {
				store.Touch(sess.ID)
			}
		}()
	}

	close(start)
	wg.Wait()
}

func TestSessionIDsAreUnique(t *testing.T) {
	store := auth.NewStore(10 * time.Minute)
	defer store.Close()

	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		sess, err := store.New()
		require.NoError(t, err)
		assert.False(t, seen[sess.ID], "duplicate session ID at iteration %d", i)
		seen[sess.ID] = true
	}
}

func TestSessionDataPersists(t *testing.T) {
	store := auth.NewStore(10 * time.Minute)
	defer store.Close()

	sess, err := store.New()
	require.NoError(t, err)
	sess.Data["key"] = "value"

	got, ok := store.Get(sess.ID)
	require.True(t, ok)
	assert.Equal(t, "value", got.Data["key"])
}

func TestCountReturnsLiveSessions(t *testing.T) {
	store := auth.NewStore(10 * time.Minute)
	defer store.Close()

	assert.Equal(t, 0, store.Count())

	a, err := store.New()
	require.NoError(t, err)
	_, err = store.New()
	require.NoError(t, err)
	assert.Equal(t, 2, store.Count())

	store.Delete(a.ID)
	assert.Equal(t, 1, store.Count())
}

func TestCountExcludesExpiredSessions(t *testing.T) {
	store := auth.NewStore(50 * time.Millisecond)
	defer store.Close()

	_, err := store.New()
	require.NoError(t, err)
	assert.Equal(t, 1, store.Count())

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, store.Count(), "expired sessions must not be counted even before cleanup runs")
}

func TestRangeVisitsLiveSessionsOnce(t *testing.T) {
	store := auth.NewStore(10 * time.Minute)
	defer store.Close()

	a, err := store.New()
	require.NoError(t, err)
	b, err := store.New()
	require.NoError(t, err)

	seen := map[string]int{}
	store.Range(func(sess *auth.Session) { seen[sess.ID]++ })
	assert.Equal(t, map[string]int{a.ID: 1, b.ID: 1}, seen)
}

func TestRangeSkipsExpiredSessions(t *testing.T) {
	store := auth.NewStore(50 * time.Millisecond)
	defer store.Close()

	_, err := store.New()
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	visited := 0
	store.Range(func(*auth.Session) { visited++ })
	assert.Equal(t, 0, visited)
}

package auth_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/auth"
)

func TestReservationsBoundConcurrentAccountAttempts(t *testing.T) {
	throttle := auth.NewThrottle()
	defer throttle.Close()
	var accepted atomic.Int32
	var wg sync.WaitGroup
	var releasesMu sync.Mutex
	var releases []func(bool)
	for range 40 {
		wg.Go(func() {
			settle, ok := throttle.Reserve("account", "ip", 5, time.Minute)
			if ok {
				accepted.Add(1)
				releasesMu.Lock()
				releases = append(releases, settle)
				releasesMu.Unlock()
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 5, accepted.Load())
	for _, settle := range releases {
		settle(true)
		settle(false)
	}
	_, ok := throttle.Reserve("account", "ip", 5, time.Minute)
	require.False(t, ok)
}

func TestSuccessDoesNotEraseFailuresOrRefundIP(t *testing.T) {
	throttle := auth.NewThrottle()
	defer throttle.Close()
	failed, ok := throttle.Reserve("account", "ip", 2, time.Minute)
	require.True(t, ok)
	success, ok := throttle.Reserve("account", "ip", 2, time.Minute)
	require.True(t, ok)
	failed(true)
	success(false)
	last, ok := throttle.Reserve("account", "ip", 2, time.Minute)
	require.True(t, ok)
	last(true)
	_, ok = throttle.Reserve("account", "ip", 2, time.Minute)
	require.False(t, ok)
	for range 5 {
		settle, ok := throttle.Reserve("other", "ip", 2, time.Minute)
		require.True(t, ok)
		settle(false)
	}
	_, ok = throttle.Reserve("third", "ip", 2, time.Minute)
	require.False(t, ok)
}

func TestSessionCapacityCountsReservations(t *testing.T) {
	store := auth.NewStore(time.Minute)
	defer store.Close()
	release, ok := store.Reserve("ip", 1, 1, 1, 1)
	require.True(t, ok)
	_, ok = store.Reserve("other", 1, 1, 1, 1)
	require.False(t, ok)
	release()
	release()
	sess, err := store.New()
	require.NoError(t, err)
	sess.Set("clientIP", "ip")
	_, ok = store.Reserve("ip", 1, 1, 1, 1)
	require.False(t, ok)
	release, ok = store.Reserve("ip", 1, 1, 1, 1, sess.ID)
	require.True(t, ok)
	release()
}

func TestPendingSessionExpiryCannotBeExtended(t *testing.T) {
	store := auth.NewStore(time.Hour)
	defer store.Close()
	sess, err := store.New()
	require.NoError(t, err)
	expiry := time.Now().Add(time.Minute)
	store.SetExpiry(sess.ID, expiry)
	store.Touch(sess.ID)
	require.Equal(t, expiry, sess.ExpiresAt)
}

package auth_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/darthsoup/goblinftp/internal/auth"
)

func TestThrottleNotThrottledInitially(t *testing.T) {
	th := auth.NewThrottle()
	assert.False(t, th.IsThrottled("user@example.com", 3))
}

func TestThrottleBlocksAfterMaxAttempts(t *testing.T) {
	th := auth.NewThrottle()
	key := "bad@example.com"
	cooldown := 5 * time.Second

	for range 3 {
		th.Record(key, cooldown)
	}
	assert.True(t, th.IsThrottled(key, 3))
}

func TestThrottleResetClearsAttempts(t *testing.T) {
	th := auth.NewThrottle()
	key := "user@example.com"
	cooldown := 5 * time.Second

	for range 3 {
		th.Record(key, cooldown)
	}
	th.Reset(key)
	assert.False(t, th.IsThrottled(key, 3))
}

func TestThrottleCooldownExpires(t *testing.T) {
	th := auth.NewThrottle()
	key := "temp@example.com"
	cooldown := 50 * time.Millisecond

	for range 3 {
		th.Record(key, cooldown)
	}
	assert.True(t, th.IsThrottled(key, 3))

	time.Sleep(100 * time.Millisecond)
	assert.False(t, th.IsThrottled(key, 3))
}

func TestThrottleIndependentKeys(t *testing.T) {
	th := auth.NewThrottle()
	cooldown := 5 * time.Second

	for range 3 {
		th.Record("bad@example.com", cooldown)
	}
	assert.True(t, th.IsThrottled("bad@example.com", 3))
	assert.False(t, th.IsThrottled("good@example.com", 3))
}

// TestThrottleEvictsExpiredEntries covers the sweeper. Without it an entry was
// only removed when that exact key was probed again after expiry, so keys that
// are never revisited accumulated forever - and the keys are attacker-supplied
// (host+username), making it an unauthenticated memory-growth path.
func TestThrottleEvictsExpiredEntries(t *testing.T) {
	thr := auth.NewThrottle()
	defer thr.Close()

	// Expires immediately; nothing ever probes this key again.
	thr.Record("abandoned-key", time.Nanosecond)
	time.Sleep(5 * time.Millisecond)

	// A fresh key forces the eviction path without probing the abandoned one.
	for i := range 20 {
		thr.Record(fmt.Sprintf("key-%d", i), time.Minute)
	}

	if thr.IsThrottled("abandoned-key", 1) {
		t.Error("an expired entry must not count as throttled")
	}
}

// TestThrottleCapsEntryCount asserts the map cannot grow without bound even
// when every key is still live, which is the shape of an attack that varies
// the username on every attempt.
func TestThrottleCapsEntryCount(t *testing.T) {
	thr := auth.NewThrottle()
	defer thr.Close()

	const attempts = 12000 // above maxThrottleEntries
	for i := range attempts {
		thr.Record(fmt.Sprintf("host:user-%d", i), time.Hour)
	}

	if n := thr.Len(); n > 10000 {
		t.Errorf("throttle held %d entries, want the cap of 10000 to hold", n)
	}
	// The cap must not break the actual throttling contract for a live key.
	thr.Record("real-target", time.Hour)
	thr.Record("real-target", time.Hour)
	thr.Record("real-target", time.Hour)
	if !thr.IsThrottled("real-target", 3) {
		t.Error("a key at maxAttempts must still throttle after eviction ran")
	}
}

// TestThrottleCloseIsIdempotent - Store.Close panicked on a second call, and
// these two sibling types should not disagree about that.
func TestThrottleCloseIsIdempotent(t *testing.T) {
	thr := auth.NewThrottle()
	thr.Close()
	thr.Close()
}

package auth

import (
	"sync"
	"time"
)

// Throttle keys are attacker-controlled (host+username, client IP), so an uncapped
// map is a memory-exhaustion path; dropping state only degrades to "not throttled".
const maxThrottleEntries = 10000

type throttleEntry struct {
	attempts  int
	expiresAt time.Time
}

// Throttle tracks per-key failed login attempts and enforces cooldown periods.
type Throttle struct {
	mu      sync.Mutex
	entries map[string]*throttleEntry
	done    chan struct{}
	stopped bool
}

// NewThrottle creates a new Throttle and starts its background sweeper.
func NewThrottle() *Throttle {
	t := &Throttle{
		entries: make(map[string]*throttleEntry),
		done:    make(chan struct{}),
	}
	go t.cleanup()
	return t
}

// Record increments the attempt counter for key and sets or extends the cooldown.
func (t *Throttle) Record(key string, cooldown time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	e := t.entries[key]
	if e == nil {
		if len(t.entries) >= maxThrottleEntries {
			t.evictLocked()
		}
		e = &throttleEntry{}
		t.entries[key] = e
	}

	e.attempts++
	e.expiresAt = time.Now().Add(cooldown)
}

// IsThrottled reports whether key has reached maxAttempts within its cooldown window.
func (t *Throttle) IsThrottled(key string, maxAttempts int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	e := t.entries[key]
	if e == nil {
		return false
	}
	if time.Now().After(e.expiresAt) {
		delete(t.entries, key)
		return false
	}

	return e.attempts >= maxAttempts
}

// Reset removes all throttle state for key.
func (t *Throttle) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

// Len returns the number of tracked entries. Exposed so tests can assert the
// entry cap actually holds.
func (t *Throttle) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

// Close stops the background sweeper. Safe to call more than once.
func (t *Throttle) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	t.stopped = true
	close(t.done)
}

// evictLocked drops expired entries, and if that frees nothing, the entries
// closest to expiry. Caller must hold mu.
func (t *Throttle) evictLocked() {
	now := time.Now()
	for k, e := range t.entries {
		if now.After(e.expiresAt) {
			delete(t.entries, k)
		}
	}
	if len(t.entries) < maxThrottleEntries {
		return
	}
	// All live: drop the soonest-expiring tenth so the map cannot wedge full.
	target := len(t.entries) - maxThrottleEntries + maxThrottleEntries/10
	for range target {
		var oldestKey string
		var oldest time.Time
		for k, e := range t.entries {
			if oldestKey == "" || e.expiresAt.Before(oldest) {
				oldestKey, oldest = k, e.expiresAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(t.entries, oldestKey)
	}
}

// cleanup drops expired entries on an interval. Without it, keys never probed
// again after expiry accumulate forever.
func (t *Throttle) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			t.mu.Lock()
			for k, e := range t.entries {
				if now.After(e.expiresAt) {
					delete(t.entries, k)
				}
			}
			t.mu.Unlock()
		case <-t.done:
			return
		}
	}
}

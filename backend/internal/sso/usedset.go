package sso

import (
	"sync"
	"time"
)

// UsedSet tracks used SSO tokens for replay protection
type UsedSet struct {
	mu      sync.Mutex
	tokens  map[string]time.Time // tokenHash -> expiry
	stopCh  chan struct{}
	stopped bool
}

// NewUsedSet creates a new UsedSet and starts the background cleanup goroutine
func NewUsedSet() *UsedSet {
	us := &UsedSet{
		tokens: make(map[string]time.Time),
		stopCh: make(chan struct{}),
	}
	go us.cleanup()
	return us
}

// Mark marks a token as used with the given expiry time
func (u *UsedSet) Mark(tokenHash string, expiry time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.tokens[tokenHash] = expiry
}

// MarkIfUnused atomically marks tokenHash used and reports whether the caller
// won the race. IsUsed followed by Mark is two lock acquisitions, so two
// simultaneous requests carrying the same one-time token both saw "unused" and
// both minted a session; only the caller that gets true here may proceed.
func (u *UsedSet) MarkIfUnused(tokenHash string, expiry time.Time) bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	if prev, exists := u.tokens[tokenHash]; exists && !time.Now().After(prev) {
		return false
	}
	u.tokens[tokenHash] = expiry
	return true
}

// IsUsed checks if a token has been used and is not expired
// Returns false if the token hasn't been marked or if it has expired
func (u *UsedSet) IsUsed(tokenHash string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	expiry, exists := u.tokens[tokenHash]
	if !exists {
		return false
	}

	// If expired, clean it up and return false
	if time.Now().After(expiry) {
		delete(u.tokens, tokenHash)
		return false
	}

	return true
}

// Stop stops the background cleanup goroutine
func (u *UsedSet) Stop() {
	u.mu.Lock()
	if u.stopped {
		u.mu.Unlock()
		return
	}
	u.stopped = true
	u.mu.Unlock()
	close(u.stopCh)
}

// cleanup runs in a background goroutine and periodically removes expired entries
func (u *UsedSet) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			u.cleanupExpired()
		case <-u.stopCh:
			return
		}
	}
}

// cleanupExpired removes all expired tokens from the set
func (u *UsedSet) cleanupExpired() {
	u.mu.Lock()
	defer u.mu.Unlock()

	now := time.Now()
	for tokenHash, expiry := range u.tokens {
		if now.After(expiry) {
			delete(u.tokens, tokenHash)
		}
	}
}

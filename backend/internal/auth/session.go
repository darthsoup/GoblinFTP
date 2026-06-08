package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Session holds per-connection state for an authenticated user.
//
// A single *Session is shared across every concurrent HTTP handler for that
// session, so its maps must never be touched directly — all access goes through
// the accessor methods, which hold mu. (mu guards data and uploads; ExpiresAt is
// guarded by the owning Store's mutex.) A concurrent read+write on a bare Go map
// is an unrecoverable runtime fatal, so this is a correctness requirement, not an
// optimisation.
//
// transferMu is a SEPARATE lock that serializes use of the one underlying
// transfer.Client: a single FTP control connection cannot service two data
// transfers at once (and jlaffaye/ftp's ServerConn is explicitly not safe for
// concurrent use), so handlers hold it around client I/O. Never acquire mu while
// holding transferMu in a way that nests inversely — the accessor methods always
// release mu before returning, so transferMu→mu is the only ordering that occurs.
type Session struct {
	ID        string
	ExpiresAt time.Time

	mu      sync.RWMutex
	data    map[string]any
	uploads map[string]any

	transferMu sync.Mutex
}

// Get returns the value stored under key, and whether it was present.
func (s *Session) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

// GetString returns the string stored under key, or "" if absent / not a string.
func (s *Session) GetString(key string) string {
	v, _ := s.Get(key)
	str, _ := v.(string)
	return str
}

// Set stores val under key.
func (s *Session) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = val
}

// Delete removes key from the session.
func (s *Session) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// PutUpload registers in-progress chunked-upload metadata under id. The uploads
// map is kept separate from data so reserve/commit handlers can mutate it under
// the same lock without a check-then-act race on the shared inner map.
func (s *Session) PutUpload(id string, meta any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.uploads == nil {
		s.uploads = make(map[string]any)
	}
	s.uploads[id] = meta
}

// GetUpload returns the upload metadata registered under id.
func (s *Session) GetUpload(id string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.uploads[id]
	return v, ok
}

// DeleteUpload removes the upload entry for id.
func (s *Session) DeleteUpload(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.uploads, id)
}

// LockTransfer acquires the per-session transfer lock. Handlers hold it around
// every operation on the transfer.Client so concurrent requests never interleave
// two data transfers on the one control connection. Pair with UnlockTransfer
// (typically via defer).
func (s *Session) LockTransfer() { s.transferMu.Lock() }

// UnlockTransfer releases the transfer lock.
func (s *Session) UnlockTransfer() { s.transferMu.Unlock() }

// TryLockTransfer reports whether the transfer lock was acquired without
// blocking. The liveness ping uses it: a transfer already in flight is itself
// proof the connection is alive, so the ping is skipped rather than queued behind
// (and corrupting) the in-flight transfer.
func (s *Session) TryLockTransfer() bool { return s.transferMu.TryLock() }

// Store is a thread-safe in-memory session store with TTL-based expiry.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
	done     chan struct{}
}

// NewStore creates a new Store with the given TTL and starts a background cleanup goroutine.
func NewStore(ttl time.Duration) *Store {
	s := &Store{
		sessions: make(map[string]*Session),
		ttl:      ttl,
		done:     make(chan struct{}),
	}
	go s.cleanup()
	return s
}

// New creates a new session with a random 16-byte hex ID.
func (s *Store) New() (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}

	sess := &Session{
		ID:        id,
		data:      make(map[string]any),
		ExpiresAt: time.Now().Add(s.ttl),
	}

	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()

	return sess, nil
}

// Get returns the session for the given ID. Returns false if not found or expired.
func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	if ok {
		ok = !time.Now().After(sess.ExpiresAt)
	}
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return sess, true
}

// Touch resets the session's expiry to now + TTL.
func (s *Store) Touch(id string) {
	s.mu.Lock()
	if sess, ok := s.sessions[id]; ok && !time.Now().After(sess.ExpiresAt) {
		sess.ExpiresAt = time.Now().Add(s.ttl)
	}
	s.mu.Unlock()
}

// Delete removes the session with the given ID.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// Count returns the number of live (non-expired) sessions.
func (s *Store) Count() int {
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, sess := range s.sessions {
		if !now.After(sess.ExpiresAt) {
			n++
		}
	}
	return n
}

// Range calls fn for each live (non-expired) session while holding a read
// lock. fn must not mutate the session or the store — read-only snapshot use
// only (e.g. the metrics collector).
func (s *Store) Range(fn func(*Session)) {
	now := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sess := range s.sessions {
		if !now.After(sess.ExpiresAt) {
			fn(sess)
		}
	}
}

// Close stops the background cleanup goroutine.
func (s *Store) Close() {
	close(s.done)
}

func (s *Store) cleanup() {
	interval := s.ttl / 2
	if interval < time.Second {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.mu.Lock()
			for id, sess := range s.sessions {
				if now.After(sess.ExpiresAt) {
					delete(s.sessions, id)
				}
			}
			s.mu.Unlock()
		case <-s.done:
			return
		}
	}
}

func newSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

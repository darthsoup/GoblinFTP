package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Session holds per-connection state for an authenticated user.
type Session struct {
	ID        string
	Data      map[string]interface{}
	ExpiresAt time.Time
}

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
		Data:      make(map[string]interface{}),
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

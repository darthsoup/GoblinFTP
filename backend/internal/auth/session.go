package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Session holds per-connection state. Reach its maps only through the accessor
// methods: a concurrent read+write on a bare Go map is an unrecoverable fatal.
type Session struct {
	ID string
	// A separate identifier for signed download URLs: those links leak into
	// history, Referer and proxy logs, where a session ID would be a takeover.
	DownloadKey string
	ExpiresAt   time.Time

	mu      sync.RWMutex
	data    map[string]any
	uploads map[string]any

	// Serializes the one transfer.Client: a single control connection cannot
	// service two transfers at once. Lock order is transferMu before mu.
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

// PutUpload registers in-progress chunked-upload metadata under id. Uploads live
// apart from data so reserve/commit avoid a check-then-act race on a shared map.
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

// UploadIDs returns the ids of this session's in-progress uploads, so a caller
// can release their staged chunks when the session goes away.
func (s *Session) UploadIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.uploads))
	for id := range s.uploads {
		ids = append(ids, id)
	}
	return ids
}

// DeleteUpload removes the upload entry for id.
func (s *Session) DeleteUpload(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.uploads, id)
}

// LockTransfer acquires the per-session transfer lock. Handlers hold it around
// every transfer.Client call so two requests never interleave on one connection.
func (s *Session) LockTransfer() { s.transferMu.Lock() }

// UnlockTransfer releases the transfer lock.
func (s *Session) UnlockTransfer() { s.transferMu.Unlock() }

// TryLockTransfer reports whether the transfer lock was acquired without blocking.
// The liveness ping skips rather than queue behind (and corrupt) a live transfer.
func (s *Session) TryLockTransfer() bool { return s.transferMu.TryLock() }

// Store is a thread-safe in-memory session store with TTL-based expiry.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	// byDownloadKey indexes the same sessions by DownloadKey. Kept in step with
	// sessions on every insert and delete.
	byDownloadKey map[string]*Session
	ttl           time.Duration
	done          chan struct{}
	stopped       bool

	// onEvict runs outside mu for each session dropped by expiry or shutdown, so
	// the store can close its FTP/SFTP connection without knowing what it is.
	onEvict func(*Session)
}

// NewStore creates a new Store with the given TTL and starts a background cleanup goroutine.
func NewStore(ttl time.Duration) *Store {
	s := &Store{
		sessions:      make(map[string]*Session),
		byDownloadKey: make(map[string]*Session),
		ttl:           ttl,
		done:          make(chan struct{}),
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

	downloadKey, err := newSessionID()
	if err != nil {
		return nil, err
	}

	sess := &Session{
		ID:          id,
		DownloadKey: downloadKey,
		data:        make(map[string]any),
		ExpiresAt:   time.Now().Add(s.ttl),
	}

	s.mu.Lock()
	s.sessions[id] = sess
	s.byDownloadKey[downloadKey] = sess
	s.mu.Unlock()

	return sess, nil
}

// GetByDownloadKey resolves a session from a download token's key. Returns
// false if not found or expired.
func (s *Store) GetByDownloadKey(key string) (*Session, bool) {
	s.mu.RLock()
	sess, ok := s.byDownloadKey[key]
	if ok {
		ok = !time.Now().After(sess.ExpiresAt)
	}
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return sess, true
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
	if sess, ok := s.sessions[id]; ok {
		delete(s.byDownloadKey, sess.DownloadKey)
	}
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

// Range calls fn for each live session while holding a read lock. fn must not
// mutate the session or the store: read-only snapshot use only.
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

// SetOnEvict registers the eviction hook under mu, since the cleanup sweep is
// already reading it. The hook must not call back into the Store: it deadlocks.
func (s *Store) SetOnEvict(fn func(*Session)) {
	s.mu.Lock()
	s.onEvict = fn
	s.mu.Unlock()
}

// Close stops the background cleanup goroutine. Safe to call more than once,
// matching sso.UsedSet.Stop.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	close(s.done)
}

// EvictExpired drops every expired session and runs onEvict for each. One
// mid-transfer is skipped, not severed; it cannot revive, so a later sweep gets it.
func (s *Store) EvictExpired() { _, _, _ = s.evict(false) }

// evictAllBudget bounds the whole shutdown sweep. A transfer wedged on an
// unresponsive remote server must not keep the process alive.
const evictAllBudget = 3 * time.Second

// EvictAll drops every session at shutdown so connections close with a QUIT,
// returning within evictAllBudget even if a transfer never finishes. It reports
// counts rather than taking a logger, which keeps package auth logging-free.
//
// clean: the transfer lock was free. forced: it was acquired after waiting.
// abandoned: the budget ran out, so the session was left for process exit.
func (s *Store) EvictAll() (clean, forced, abandoned int) { return s.evict(true) }

func (s *Store) evict(all bool) (clean, forced, abandoned int) {
	now := time.Now()

	s.mu.RLock()
	onEvict := s.onEvict
	victims := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		if all || now.After(sess.ExpiresAt) {
			victims = append(victims, sess)
		}
	}
	s.mu.RUnlock()

	deadline := time.Now().Add(evictAllBudget)
	for i, sess := range victims {
		// The hook closes a connection and may talk to the chunk store, so the
		// budget has to cover the whole loop, not only the lock wait.
		if all && time.Now().After(deadline) {
			abandoned += len(victims) - i
			break
		}
		locked := sess.TryLockTransfer()
		wasBusy := !locked
		if !locked {
			if !all {
				// Busy: a transfer is proof of life. Collect it next tick
				// rather than truncating a download.
				continue
			}
			// Wait for the transfer, but never indefinitely: a download holds
			// this lock for its whole stream and would hang us until SIGKILL.
			locked = waitForTransfer(sess, deadline)
		}
		if onEvict != nil {
			onEvict(sess)
		}
		if locked {
			sess.UnlockTransfer()
		}
		switch {
		case !locked:
			abandoned++
		case wasBusy:
			forced++
		default:
			clean++
		}

		s.mu.Lock()
		delete(s.byDownloadKey, sess.DownloadKey)
		delete(s.sessions, sess.ID)
		s.mu.Unlock()
	}
	return clean, forced, abandoned
}

// waitForTransfer polls for the session's transfer lock until deadline. Polling
// rather than blocking because sync.Mutex has no timed acquire.
func waitForTransfer(sess *Session, deadline time.Time) bool {
	for time.Now().Before(deadline) {
		if sess.TryLockTransfer() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
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
			s.EvictExpired()
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

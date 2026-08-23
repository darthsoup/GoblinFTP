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
// session, so its maps must never be touched directly - all access goes through
// the accessor methods, which hold mu. (mu guards data and uploads; ExpiresAt is
// guarded by the owning Store's mutex.) A concurrent read+write on a bare Go map
// is an unrecoverable runtime fatal, so this is a correctness requirement, not an
// optimisation.
//
// transferMu is a SEPARATE lock that serializes use of the one underlying
// transfer.Client: a single FTP control connection cannot service two data
// transfers at once (and jlaffaye/ftp's ServerConn is explicitly not safe for
// concurrent use), so handlers hold it around client I/O. Never acquire mu while
// holding transferMu in a way that nests inversely - the accessor methods always
// release mu before returning, so transferMu→mu is the only ordering that occurs.
type Session struct {
	ID string
	// DownloadKey is a second, independent random identifier used only in
	// signed download URLs. Download links travel where the HttpOnly session
	// cookie deliberately does not (browser history, Referer, proxy logs,
	// "copy link"), so embedding the session ID there made any leaked link a
	// full session takeover. This value is useless as a cookie.
	DownloadKey string
	ExpiresAt   time.Time

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
	// byDownloadKey indexes the same sessions by DownloadKey. Kept in step with
	// sessions on every insert and delete.
	byDownloadKey map[string]*Session
	ttl           time.Duration
	done          chan struct{}
	stopped       bool

	// onEvict runs for each session dropped by expiry or shutdown, always
	// outside mu. It exists so the store can release a session's resources
	// (its FTP/SFTP connection) without this package knowing what they are.
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

// Range calls fn for each live (non-expired) session while holding a read
// lock. fn must not mutate the session or the store - read-only snapshot use
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

// SetOnEvict registers the eviction hook. Guarded by mu: the cleanup goroutine
// is already running by the time wiring calls this, so an unguarded write here
// races with the sweep that reads it.
//
// The hook must not call back into the Store (Touch, Delete, Get, Evict*): it
// runs on a path that re-acquires mu, so a re-entrant call self-deadlocks.
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

// EvictExpired drops every expired session and runs onEvict for each.
//
// A session mid-transfer is skipped rather than torn down: transferMu is held
// for the length of a transfer, and closing the connection under it would sever
// a running download. It cannot come back to life (Touch refuses an expired
// session and Get reports it missing), so the next sweep collects it.
func (s *Store) EvictExpired() { s.evict(false) }

// evictAllBudget bounds the whole shutdown sweep. A transfer wedged on an
// unresponsive remote server must not keep the process alive.
const evictAllBudget = 5 * time.Second

// EvictAll drops every session regardless of expiry. Used at shutdown, after
// the HTTP server has drained, to close connections that would otherwise be
// severed without a QUIT. It returns within evictAllBudget even if a transfer
// never finishes, closing that session's client anyway: the process is exiting
// either way, and a clean close beats waiting for SIGKILL.
func (s *Store) EvictAll() { s.evict(true) }

func (s *Store) evict(all bool) {
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
	for _, sess := range victims {
		// The hook closes a connection and may talk to the chunk store, so the
		// budget has to cover the whole loop, not only the lock wait.
		if all && time.Now().After(deadline) {
			break
		}
		locked := sess.TryLockTransfer()
		if !locked {
			if !all {
				// Busy: a transfer is proof of life. Collect it next tick
				// rather than truncating a download.
				continue
			}
			// Shutdown. Wait a little for the transfer to finish, but never
			// indefinitely: /api/files/download holds this lock for a whole
			// stream and Shutdown does not kill its goroutine, so blocking
			// here would hang the process until SIGKILL.
			locked = waitForTransfer(sess, deadline)
		}
		if onEvict != nil {
			onEvict(sess)
		}
		if locked {
			sess.UnlockTransfer()
		}

		s.mu.Lock()
		delete(s.byDownloadKey, sess.DownloadKey)
		delete(s.sessions, sess.ID)
		s.mu.Unlock()
	}
}

// waitForTransfer polls for the session's transfer lock until deadline,
// reporting whether it was acquired. Polling rather than blocking because
// sync.Mutex has no timed acquire.
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

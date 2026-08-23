// backend/internal/staging/sweeper.go
package staging

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// staleAfter is how long an untouched upload directory must sit before the
// sweeper reclaims it. Creating each chunk adds a directory entry, which bumps
// the directory's mtime, so an upload in progress can never look stale. Hours
// rather than minutes because a client may legitimately pause between the
// reserve and the commit (a conflict prompt waits on the user).
const staleAfter = 24 * time.Hour

// sweepInterval is deliberately coarse: this reclaims disk, it is not on any
// request path.
const sweepInterval = time.Hour

// chunkNameRe matches the %04d chunk files transfer.WriteChunk writes.
var chunkNameRe = regexp.MustCompile(`^\d{4,}$`)

// Sweeper reclaims local staging directories that no longer belong to any
// upload. Handlers clean up on commit and abort, but an abandoned reservation
// (closed tab, dropped connection, or a restart, which loses the in-memory
// session store entirely) leaves its chunks behind forever.
//
// S3 staging is not swept: reaping a remote bucket on a timer is the bucket's
// job. See docs/s3-staging.md for the lifecycle rule.
type Sweeper struct {
	root   string
	logger *slog.Logger
	// inUse reports whether an upload is still referenced by a live session.
	// The mtime guard alone is not sufficient: the session TTL has no upper
	// bound, and WriteChunk uses os.Create, which truncates an existing chunk
	// name without adding a directory entry, so a client retrying indices it
	// already wrote does not refresh the directory's mtime.
	inUse func(uploadID string) bool

	mu      sync.Mutex
	done    chan struct{}
	stopped bool
}

// NewSweeper starts a sweeper over dataDir. inUse may be nil, in which case
// only the age and shape guards apply. It runs one pass immediately, which is
// the pass that matters after a restart.
func NewSweeper(dataDir string, logger *slog.Logger, inUse func(string) bool) *Sweeper {
	s := newSweeper(dataDir, logger, inUse)
	go s.run()
	return s
}

// newSweeper builds a sweeper without starting its goroutine.
func newSweeper(dataDir string, logger *slog.Logger, inUse func(string) bool) *Sweeper {
	return &Sweeper{root: dataDir, logger: logger, inUse: inUse, done: make(chan struct{})}
}

// Close stops the sweeper. Safe to call more than once.
func (s *Sweeper) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	close(s.done)
}

func (s *Sweeper) run() {
	s.Sweep()
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.Sweep()
		case <-s.done:
			return
		}
	}
}

// Sweep removes every stale upload directory under the root. Exported so tests
// can drive a pass without waiting on the ticker.
func (s *Sweeper) Sweep() {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		// A missing or unreadable data dir is the operator's problem and is
		// already loud at startup; nothing to reclaim here either way.
		return
	}
	cutoff := time.Now().Add(-staleAfter)
	for _, entry := range entries {
		if !s.isReclaimable(entry, cutoff) {
			continue
		}
		// Through transfer.Cleanup, not os.RemoveAll: it re-applies the upload
		// ID guard inside the delete.
		if err := transfer.Cleanup(s.root, entry.Name()); err != nil {
			s.logger.Warn("could not reclaim stale upload staging directory",
				"upload_id", entry.Name(), "error", err.Error())
			continue
		}
		s.logger.Info("reclaimed stale upload staging directory", "upload_id", entry.Name())
	}
}

// isReclaimable holds three independent guards, all of which must pass. The
// data dir also holds known_hosts and themes/, so a single wrong guess here
// would delete an operator's data.
func (s *Sweeper) isReclaimable(entry os.DirEntry, cutoff time.Time) bool {
	if !entry.IsDir() {
		return false
	}
	// Only ever a directory this package created: the name must be an upload ID.
	if transfer.ValidateUploadID(entry.Name()) != nil {
		return false
	}
	info, err := entry.Info()
	if err != nil || info.ModTime().After(cutoff) {
		return false
	}
	if s.inUse != nil && s.inUse(entry.Name()) {
		return false
	}
	return s.holdsOnlyChunks(filepath.Join(s.root, entry.Name()))
}

// holdsOnlyChunks reports whether dir contains nothing but chunk files, so a
// UUID-named directory that is not ours is left alone.
func (s *Sweeper) holdsOnlyChunks(dir string) bool {
	children, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, child := range children {
		if child.IsDir() || !chunkNameRe.MatchString(strings.TrimSpace(child.Name())) {
			return false
		}
	}
	return true
}

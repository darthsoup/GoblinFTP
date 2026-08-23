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

// staleAfter: creating a chunk bumps the directory mtime, so an upload in
// progress never looks stale. Hours, since a conflict prompt can pause a client.
const staleAfter = 24 * time.Hour

// sweepInterval is deliberately coarse: this reclaims disk, it is not on any
// request path.
const sweepInterval = time.Hour

// chunkNameRe matches the %04d chunk files transfer.WriteChunk writes.
var chunkNameRe = regexp.MustCompile(`^\d{4,}$`)

// Sweeper reclaims local staging directories left by abandoned reservations
// (a restart loses the in-memory sessions). S3 is not swept: see docs/s3-staging.md.
type Sweeper struct {
	root   string
	logger *slog.Logger
	// inUse reports whether a live session still references the upload. The mtime
	// guard alone misses retries: os.Create truncates without touching the mtime.
	inUse func(uploadID string) bool

	mu      sync.Mutex
	done    chan struct{}
	stopped bool
}

// NewSweeper starts a sweeper over dataDir; a nil inUse leaves only the age and
// shape guards. One pass runs immediately, the pass that matters after a restart.
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

// isReclaimable holds independent guards, all of which must pass: the data dir
// also holds known_hosts and themes/, so a wrong guess deletes operator data.
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

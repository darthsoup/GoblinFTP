package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/darthsoup/goblinftp/internal/auth"
	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

var errStorageLimit = errors.New("temporary storage quota exceeded")

type resourceBudget struct {
	reservedChunks int
	remote         interface {
		StagedBytes(context.Context, map[string]bool) (int64, error)
	}
	remoteBytes           int64
	remoteChecked         time.Time
	uploads               map[string]bool
	mu                    sync.Mutex
	used, max, perSession int64
	owners                map[string]int64
	orphans               map[string]int64
	scanErr               error
}
type diskQuota struct {
	bytesLeft int64
	remaining int
	ctx       context.Context
	budget    *resourceBudget
	owner     string
}

func newResourceBudget(dir string, max, perSession int64) *resourceBudget {
	b := &resourceBudget{max: max, perSession: perSession, owners: make(map[string]int64), orphans: make(map[string]int64)}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return b
	}
	if err != nil {
		b.scanErr = err
		return b
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "gftp-") && transfer.ValidateUploadID(e.Name()) != nil {
			continue
		}
		root := filepath.Join(dir, e.Name())
		var rootBytes int64
		err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				info, err := d.Info()
				if err != nil {
					return err
				}
				rootBytes += info.Size()
			}
			return nil
		})
		b.orphans[root] = rootBytes
		b.used += rootBytes
		if err != nil {
			b.scanErr = err
		}
	}
	return b
}

func (b *resourceBudget) reserve(owner string, size int64) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.remote != nil && time.Since(b.remoteChecked) > time.Minute {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		bytes, err := b.remote.StagedBytes(ctx, b.uploads)
		cancel()
		if err != nil {
			return nil, errStorageLimit
		}
		b.used += bytes - b.remoteBytes
		b.remoteBytes = bytes
		b.remoteChecked = time.Now()
	}
	for p, size := range b.orphans {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			b.used -= size
			delete(b.orphans, p)
		}
	}
	if b.scanErr != nil || size < 0 || size > b.max-b.used || size > b.perSession-b.owners[owner] {
		return nil, errStorageLimit
	}
	b.used += size
	b.owners[owner] += size
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.used -= size
			b.owners[owner] -= size
			if b.owners[owner] == 0 {
				delete(b.owners, owner)
			}
		})
	}, nil
}

func (h *Handler) quota(c echo.Context) *diskQuota {
	sess, _ := c.Get("session").(*auth.Session)
	owner := c.RealIP()
	if sess != nil {
		owner = sess.ID
	}
	return &diskQuota{budget: h.resources, owner: owner, remaining: 100000, bytesLeft: maxZipSize, ctx: c.Request().Context()}
}

type quotaTemp struct {
	*os.File
	quota    *diskQuota
	releases []func()
	written  int64
}

func newQuotaTemp(dir, pattern string, quotas ...*diskQuota) (*quotaTemp, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	t := &quotaTemp{File: f}
	if len(quotas) > 0 {
		t.quota = quotas[0]
	}
	return t, nil
}
func (t *quotaTemp) Write(p []byte) (int, error) {
	if t.quota != nil && t.quota.ctx != nil {
		if err := t.quota.ctx.Err(); err != nil {
			return 0, err
		}
	}
	if int64(len(p)) > maxZipSize-t.written {
		return 0, errStorageLimit
	}
	if t.quota != nil && t.quota.budget != nil {
		release, err := t.quota.budget.reserve(t.quota.owner, int64(len(p)))
		if err != nil {
			return 0, err
		}
		t.releases = append(t.releases, release)
	}
	n, err := t.File.Write(p)
	t.written += int64(n)
	return n, err
}
func (t *quotaTemp) dispose() {
	_ = t.Close()
	if err := os.Remove(t.Name()); err == nil || os.IsNotExist(err) {
		for _, release := range t.releases {
			release()
		}
	}
}

func (h *Handler) resourceMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		p := c.Request().URL.Path
		expensive := p == "/api/files" && c.Request().Method == http.MethodGet || (p == "/api/files/extract" || p == "/api/files/compress" || p == "/api/files/copy") || p == "/api/files/download-zip"
		if expensive {
			select {
			case h.expensive <- struct{}{}:
				defer func() { <-h.expensive }()
			default:
				return Fail(c, gftperrors.New(gftperrors.ErrCapacity, "operation capacity reached"))
			}
		}
		if strings.HasPrefix(c.Request().Header.Get("Content-Type"), "multipart/") {
			sess, ok := lookupSession(c, h.store)
			if !ok {
				return Fail(c, gftperrors.New(gftperrors.ErrUnauthorized, "no active session"))
			}
			size := c.Request().ContentLength
			if size < 0 {
				size = maxZipSize + 32<<20
				if p != "/api/files/extract" {
					size = h.cfg.ChunkSize + 8<<20
				}
			}
			release, err := h.resources.reserve(sess.ID, size)
			if err != nil {
				return Fail(c, gftperrors.New(gftperrors.ErrCapacity, "temporary storage quota exceeded"))
			}
			defer func() {
				if form := c.Request().MultipartForm; form != nil {
					if err := form.RemoveAll(); err != nil {
						return
					}
				}
				release()
			}()
		}
		return next(c)
	}
}

// Context cancellation is checked even when the remote reader keeps producing data.
type contextReader struct {
	ctx interface{ Err() error }
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func (b *resourceBudget) ownUpload(id string, release func()) func() {
	b.mu.Lock()
	if b.uploads == nil {
		b.uploads = make(map[string]bool)
	}
	b.uploads[id] = true
	b.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { release(); b.mu.Lock(); delete(b.uploads, id); b.mu.Unlock() }) }
}

func (q *diskQuota) visit() error {
	if q.ctx != nil {
		if err := q.ctx.Err(); err != nil {
			return err
		}
	}
	q.remaining--
	if q.remaining < 0 {
		return errTreeTooDeep
	}
	return nil
}

func (t *quotaTemp) ReadFrom(r io.Reader) (int64, error) { return io.Copy(struct{ io.Writer }{t}, r) }

func (t *quotaTemp) WriteString(value string) (int, error) { return t.Write([]byte(value)) }

func (b *resourceBudget) reserveUpload(owner string, size int64, chunks int) (func(), error) {
	release, err := b.reserve(owner, size)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	if chunks > maxTotalChunks-b.reservedChunks {
		b.mu.Unlock()
		release()
		return nil, errStorageLimit
	}
	b.reservedChunks += chunks
	b.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { release(); b.mu.Lock(); b.reservedChunks -= chunks; b.mu.Unlock() }) }, nil
}

package transfer

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"path"
	"strings"
	"time"
)

type DirectorySizeResult struct {
	Path           string `json:"path"`
	Bytes          int64  `json:"bytes"`
	FileCount      int    `json:"fileCount"`
	DirectoryCount int    `json:"directoryCount"`
	HasEntries     bool   `json:"hasEntries"`
	Complete       bool   `json:"complete"`
	SkippedCount   int    `json:"skippedCount"`
	CalculatedAt   string `json:"calculatedAt"`
	Reason         string `json:"reason,omitempty"`
}

type DirectorySizeReader interface {
	LstatContext(context.Context, string) (FileInfo, error)
	ReadDirContext(context.Context, string, int) ([]FileInfo, bool, error)
}

// ErrDirectorySizeUnavailable aborts a scan whose session was disconnected or replaced.
var ErrDirectorySizeUnavailable = errors.New("directory size session unavailable")

var ErrDirectorySizeLimit = errors.New("directory size metadata limit reached")

type DirectorySizeOptions struct {
	MaxDepth   int
	MaxEntries int
	Timeout    time.Duration
}

// ValidateDirectorySizePath rejects traversal and command delimiters before any remote call.
func ValidateDirectorySizePath(p string) bool {
	if !path.IsAbs(p) || strings.ContainsAny(p, "\\\x00\r\n") {
		return false
	}
	for part := range strings.SplitSeq(p, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func CalculateDirectorySize(ctx context.Context, client DirectorySizeReader, root string, options ...DirectorySizeOptions) (DirectorySizeResult, error) {
	limits := DirectorySizeOptions{MaxDepth: 64, MaxEntries: 50000, Timeout: 30 * time.Second}
	if len(options) > 0 {
		if options[0].MaxDepth > 0 {
			limits.MaxDepth = options[0].MaxDepth
		}
		if options[0].MaxEntries > 0 {
			limits.MaxEntries = options[0].MaxEntries
		}
		if options[0].Timeout > 0 {
			limits.Timeout = options[0].Timeout
		}
	}
	return calculateDirectorySize(ctx, client, root, limits)
}

func calculateDirectorySize(ctx context.Context, client DirectorySizeReader, root string, limits DirectorySizeOptions) (result DirectorySizeResult, err error) {
	result.Path = path.Clean(root)
	result.Complete = true
	defer func() { result.CalculatedAt = time.Now().UTC().Format(time.RFC3339) }()
	if !ValidateDirectorySizePath(root) {
		return result, ErrInvalidType
	}
	scanCtx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()
	walker := directorySizeWalker{ctx: scanCtx, caller: ctx, client: client, limits: limits, result: &result}
	if stop, err := walker.stopped(); stop {
		return result, err
	}
	info, err := client.LstatContext(scanCtx, result.Path)
	if directorySizeFatal(err) {
		return result, err
	}
	if stop, ctxErr := walker.stopped(); stop {
		return result, ctxErr
	}
	if err != nil {
		if errors.Is(err, ErrDirectorySizeLimit) {
			walker.skip("limit", 1)
			return result, nil
		}
		return result, err
	}
	if !info.IsDir || info.IsSymlink {
		return result, ErrInvalidType
	}
	err = walker.walk(result.Path, 0)
	return result, err
}

type directorySizeWalker struct {
	ctx     context.Context
	caller  context.Context
	client  DirectorySizeReader
	limits  DirectorySizeOptions
	result  *DirectorySizeResult
	entries int
}

func (w *directorySizeWalker) skip(reason string, count int) {
	w.result.Complete = false
	w.result.SkippedCount += count
	if w.result.Reason == "" || reason == "timeout" || reason == "limit" && w.result.Reason == "unreadable" {
		w.result.Reason = reason
	}
}

func (w *directorySizeWalker) stopped() (bool, error) {
	if err := w.caller.Err(); err != nil {
		return true, err
	}
	select {
	case <-w.ctx.Done():
		w.skip("timeout", 1)
		return true, nil
	default:
		return false, nil
	}
}

func (w *directorySizeWalker) walk(dir string, depth int) error {
	if stop, err := w.stopped(); stop {
		return err
	}
	remaining := w.limits.MaxEntries - w.entries
	if remaining <= 0 {
		w.skip("limit", 1)
		return nil
	}
	entries, truncated, err := w.client.ReadDirContext(w.ctx, dir, remaining)
	if directorySizeFatal(err) {
		return err
	}
	if stop, ctxErr := w.stopped(); stop {
		return ctxErr
	}
	if err != nil {
		if errors.Is(err, ErrDirectorySizeLimit) {
			w.skip("limit", 1)
			return nil
		}
		if depth == 0 {
			return err
		}
		w.skip("unreadable", 1)
		return nil
	}
	if len(entries) > remaining {
		entries = entries[:remaining]
		truncated = true
	}
	w.entries += len(entries)
	if truncated {
		w.skip("limit", 1)
	}
	seen := make(map[string]bool)
	for _, entry := range entries {
		if stop, err := w.stopped(); stop {
			return err
		}
		if entry.Name == "." || entry.Name == ".." {
			continue
		}
		if depth == 0 {
			w.result.HasEntries = true
		}
		if entry.IsSymlink {
			continue
		}
		child := path.Join(dir, entry.Name)
		if entry.Name == "" || strings.ContainsAny(entry.Name, "/\\\x00\r\n") || seen[entry.Name] ||
			child == w.result.Path || !strings.HasPrefix(child, strings.TrimSuffix(w.result.Path, "/")+"/") {
			w.skip("unreadable", 1)
			continue
		}
		seen[entry.Name] = true
		if entry.IsDir {
			w.result.DirectoryCount++
			if depth >= w.limits.MaxDepth {
				w.skip("limit", 1)
				continue
			}
			if err := w.walk(child, depth+1); err != nil {
				return err
			}
			if w.result.Reason == "timeout" {
				return nil
			}
			continue
		}
		if entry.Size < 0 {
			w.skip("unreadable", 1)
			continue
		}
		if entry.Size > math.MaxInt64-w.result.Bytes {
			w.skip("limit", 1)
			continue
		}
		w.result.Bytes += entry.Size
		w.result.FileCount++
	}
	return nil
}

func directorySizeFatal(err error) bool {
	return errors.Is(err, ErrDirectorySizeUnavailable) || errors.Is(err, ErrConnectionLost) || errors.Is(err, ErrConnectionTimeout) ||
		errors.Is(err, ErrConnectionFailed) || errors.Is(err, ErrAuthFailed) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

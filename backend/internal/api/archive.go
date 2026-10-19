package api

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/metrics"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

const maxZipSize = 512 * 1024 * 1024 // 512 MB

// maxTreeDepth bounds every recursive walk of a remote directory tree: a symlink
// loop otherwise exhausts the goroutine stack, a crash middleware.Recover cannot catch.
const maxTreeDepth = 64

// errTreeTooDeep is returned once maxTreeDepth is exceeded.
var errTreeTooDeep = errors.New("directory tree is nested too deeply")

// errArchiveTooLarge is returned once an extraction exceeds its budget.
var errArchiveTooLarge = errors.New("archive expands beyond the maximum extracted size")

// errArchiveEscape marks a zip-slip entry, and errArchiveCorrupt a truncated or
// malformed archive. Both are the caller's payload, never a server fault, so
// they must be classified before failClient sees them.
var (
	errArchiveEscape  = errors.New("archive entry escapes the destination")
	errArchiveCorrupt = errors.New("archive is truncated or corrupt")
)

// archiveError maps an extraction failure to its API error. Ordered most
// specific first; anything unmatched belongs to the remote server.
func archiveError(err error) *gftperrors.GFTPError {
	switch {
	case errors.Is(err, errArchiveCorrupt):
		return gftperrors.New(gftperrors.ErrArchiveFormat,
			"the archive is truncated or corrupt").WithCause(err)
	case errors.Is(err, errArchiveEscape):
		return gftperrors.New(gftperrors.ErrArchiveFormat,
			"the archive contains an entry that would write outside the destination").WithCause(err)
	case errors.Is(err, errArchiveTooLarge):
		return gftperrors.New(gftperrors.ErrFileTooLarge,
			"the archive expands beyond the maximum extracted size").WithCause(err)
	case errors.Is(err, errTreeTooDeep):
		return gftperrors.New(gftperrors.ErrBadRequest,
			"the archive is nested too deeply").WithCause(err)
	}
	return nil
}

// extractBudget caps the total DECOMPRESSED bytes an extraction may write to the
// remote. maxZipSize bounds only the compressed upload: 1 MB of bz2 expands to GBs.
type extractBudget struct {
	remaining int64
	entries   int
	ctx       context.Context
}

func newExtractBudget(contexts ...context.Context) *extractBudget {
	ctx := context.Background()
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	return &extractBudget{remaining: maxZipSize, ctx: ctx}
}
func (b *extractBudget) next() error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	b.entries++
	if b.entries > maxArchiveEntries {
		return errArchiveCorrupt
	}
	return nil
}

// wrap returns r limited to the budget still available, decrementing as it is
// consumed. Reading past the budget fails the whole extraction.
func (b *extractBudget) wrap(r io.Reader) io.Reader { return &budgetReader{b: b, r: r} }

type budgetReader struct {
	b *extractBudget
	r io.Reader
}

func (br *budgetReader) Read(p []byte) (int, error) {
	if err := br.b.ctx.Err(); err != nil {
		return 0, err
	}
	if br.b.remaining <= 0 {
		return 0, errArchiveTooLarge
	}
	if int64(len(p)) > br.b.remaining {
		p = p[:br.b.remaining]
	}
	n, err := br.r.Read(p)
	br.b.remaining -= int64(n)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, tagCorrupt(err)
	}
	return n, err
}

// tagCorrupt marks the reader-side failures that mean a bad payload rather than
// a dead connection. io.ErrUnexpectedEOF in particular used to reach isConnLost
// and close a perfectly healthy session.
func tagCorrupt(err error) error {
	if errors.Is(err, errArchiveTooLarge) || errors.Is(err, errArchiveCorrupt) {
		return err
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, gzip.ErrChecksum) ||
		errors.Is(err, gzip.ErrHeader) || errors.Is(err, tar.ErrHeader) ||
		errors.Is(err, zip.ErrFormat) || errors.Is(err, zip.ErrChecksum) {
		return fmt.Errorf("%w: %w", errArchiveCorrupt, err)
	}
	return err
}

// safeJoin joins destination and name, returning an error if the result escapes destination.
func safeJoin(destination, name string) (string, error) {
	if name == "" || path.IsAbs(name) || strings.ContainsAny(name, "\\\x00\r\n") || transfer.HasDrivePrefix(name) {
		return "", fmt.Errorf("%w: %q", errArchiveEscape, name)
	}
	if len(strings.Split(name, "/")) > 64 {
		return "", errArchiveEscape
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return "", fmt.Errorf("%w: %q", errArchiveEscape, name)
		}
	}
	outPath := path.Clean(path.Join(destination, name))
	cleanDest := path.Clean(destination)
	prefix := strings.TrimSuffix(cleanDest, "/") + "/"
	if outPath != cleanDest && !strings.HasPrefix(outPath, prefix) {
		return "", fmt.Errorf("%w: %q", errArchiveEscape, name)
	}
	return outPath, nil
}

// ExtractArchive extracts an uploaded archive (zip/tar/tar.gz/tar.bz2) to a remote destination.
func (h *Handler) ExtractArchive(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	destination := c.FormValue("destination")
	if !validPath(c, destination, true) {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "destination is required"))
	}
	fh, err := c.FormFile("archive")
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "archive file is required"))
	}
	f, err := fh.Open()
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "failed to open archive"))
	}
	defer f.Close()

	filename := strings.ToLower(fh.Filename)
	switch {
	case strings.HasSuffix(filename, ".zip"):
		if fh.Size > maxZipSize {
			return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "archive exceeds maximum size"))
		}
		// multipart.File is an io.ReaderAt, all zip.NewReader wants, so the archive
		// is never held in memory (net/http already spooled it past its threshold).
		if err := preflightZip(f, fh.Size); err != nil {
			return Fail(c, archiveError(err))
		}
		zr, zipErr := zip.NewReader(f, fh.Size)
		if zipErr != nil {
			return Fail(c, gftperrors.New(gftperrors.ErrArchiveFormat, "invalid zip archive").WithCause(zipErr))
		}
		written, skipped, err := extractZip(client, zr, destination, newExtractBudget(c.Request().Context()))
		if err != nil {
			if gerr := archiveError(err); gerr != nil {
				return Fail(c, gerr)
			}
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		return OK(c, extractResult{Written: written, Skipped: skipped})
	case strings.HasSuffix(filename, ".tar.gz") || strings.HasSuffix(filename, ".tgz"):
		gr, gzErr := gzip.NewReader(f)
		if gzErr != nil {
			return Fail(c, gftperrors.New(gftperrors.ErrArchiveFormat, "invalid gzip archive").WithCause(gzErr))
		}
		defer gr.Close()
		written, skipped, err := extractTar(client, tar.NewReader(&archiveInput{r: gr, remaining: maxZipSize, ctx: c.Request().Context()}), destination, newExtractBudget(c.Request().Context()))
		if err != nil {
			if gerr := archiveError(err); gerr != nil {
				return Fail(c, gerr)
			}
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		return OK(c, extractResult{Written: written, Skipped: skipped})
	case strings.HasSuffix(filename, ".tar.bz2"):
		written, skipped, err := extractTar(client, tar.NewReader(&archiveInput{r: bzip2.NewReader(f), remaining: maxZipSize, ctx: c.Request().Context()}), destination, newExtractBudget(c.Request().Context()))
		if err != nil {
			if gerr := archiveError(err); gerr != nil {
				return Fail(c, gerr)
			}
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		return OK(c, extractResult{Written: written, Skipped: skipped})
	case strings.HasSuffix(filename, ".tar"):
		written, skipped, err := extractTar(client, tar.NewReader(&archiveInput{r: f, remaining: maxZipSize, ctx: c.Request().Context()}), destination, newExtractBudget(c.Request().Context()))
		if err != nil {
			if gerr := archiveError(err); gerr != nil {
				return Fail(c, gerr)
			}
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		return OK(c, extractResult{Written: written, Skipped: skipped})
	default:
		return Fail(c, gftperrors.New(gftperrors.ErrArchiveFormat, "unsupported archive format"))
	}
}

// extractResult reports what an extraction actually did. Entries an archive
// format allows but SFTP/FTP cannot represent (symlinks, devices) are skipped
// rather than failing the whole upload, so the caller has to be told.
type extractResult struct {
	Written int            `json:"written"`
	Skipped []skippedEntry `json:"skipped"`
}

type skippedEntry struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func extractZip(client transfer.Client, zr *zip.Reader, destination string, budget *extractBudget) (int, []skippedEntry, error) {
	written := 0
	skipped := []skippedEntry{}
	for _, entry := range zr.File {
		if err := budget.next(); err != nil {
			return written, skipped, err
		}
		outPath, err := safeJoin(destination, entry.Name)
		if err != nil {
			return written, skipped, err
		}
		if budget.remaining < 0 || entry.UncompressedSize64 > uint64(budget.remaining) {
			return written, skipped, errArchiveTooLarge
		}
		if entry.FileInfo().IsDir() {
			if err := ensureDirAll(client, outPath); err != nil {
				return written, skipped, err
			}
			continue
		}
		if !entry.FileInfo().Mode().IsRegular() {
			skipped = append(skipped, skippedEntry{Name: entry.Name, Reason: "not a regular file"})
			continue
		}
		if err := ensureDirAll(client, path.Dir(outPath)); err != nil {
			return written, skipped, err
		}
		rc, err := entry.Open()
		if err != nil {
			return written, skipped, tagCorrupt(err)
		}
		err = client.Upload(outPath, budget.wrap(rc))
		_ = rc.Close()
		if err != nil {
			return written, skipped, err
		}
		written++
	}
	return written, skipped, nil
}

func extractTar(client transfer.Client, tr *tar.Reader, destination string, budget *extractBudget) (int, []skippedEntry, error) {
	written := 0
	skipped := []skippedEntry{}
	for {
		if err := budget.next(); err != nil {
			return written, skipped, err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return written, skipped, tagCorrupt(err)
		}
		outPath, err := safeJoin(destination, hdr.Name)
		if err != nil {
			return written, skipped, err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := ensureDirAll(client, outPath); err != nil {
				return written, skipped, err
			}
		case tar.TypeReg:
			if hdr.Size > budget.remaining {
				return written, skipped, errArchiveTooLarge
			}
			if err := ensureDirAll(client, path.Dir(outPath)); err != nil {
				return written, skipped, err
			}
			if err := client.Upload(outPath, budget.wrap(tr)); err != nil {
				return written, skipped, err
			}
			written++
		case tar.TypeXHeader, tar.TypeXGlobalHeader:
			// Metadata records, not entries: silently correct to ignore.
		default:
			skipped = append(skipped, skippedEntry{
				Name:   hdr.Name,
				Reason: tarTypeName(hdr.Typeflag),
			})
		}
	}
	return written, skipped, nil
}

func tarTypeName(flag byte) string {
	switch flag {
	case tar.TypeSymlink:
		return "symlink"
	case tar.TypeLink:
		return "hard link"
	case tar.TypeChar, tar.TypeBlock:
		return "device node"
	case tar.TypeFifo:
		return "fifo"
	default:
		return "unsupported entry type"
	}
}

// CreateZip downloads the given remote paths and uploads a new zip to destination.
func (h *Handler) CreateZip(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()
	var req struct {
		Paths       []string `json:"paths"`
		Destination string   `json:"destination"`
	}
	if gerr := bindJSON(c, &req); gerr != nil {
		return Fail(c, gerr)
	}
	if len(req.Paths) == 0 || req.Destination == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "paths and destination are required"))
	}

	// Bounded before anything is written: the archive is spooled to the data
	// dir, so an unbounded input would fill the volume.
	if !validPath(c, req.Destination, false) {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "invalid archive destination"))
	}
	for _, p := range req.Paths {
		if !validPath(c, p, false) {
			return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "invalid archive source"))
		}
	}
	walk := h.quota(c)
	var totalSize int64
	for _, p := range req.Paths {
		size, err := zipInputSizeDepth(client, p, 0, walk)
		if err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
		totalSize += size
		if totalSize > maxZipSize {
			return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "archive exceeds maximum size"))
		}
	}

	// Spooled to disk, not piped into the upload: FTP allows one data transfer per
	// control connection, so a pipe desynced RETR against STOR and could deadlock.
	tmp, err := newQuotaTemp(h.cfg.DataDir, "gftp-zip-*", h.quota(c))
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "could not stage archive").WithCause(err))
	}
	defer func() {
		_ = tmp.Close()
		tmp.dispose()
	}()

	zw := zip.NewWriter(tmp)
	walk = h.quota(c)
	for _, p := range req.Paths {
		if err := addToZipDepth(zw, client, p, "", nil, 0, walk); err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
	}
	// A failed finalization must not reach the upload: it would write a truncated
	// archive and report success.
	if err := zw.Close(); err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "failed to finalize archive").WithCause(err))
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "failed to rewind archive").WithCause(err))
	}
	if err := client.Upload(req.Destination, tmp); err != nil {
		return failClient(c, gftperrors.ErrOperationFailed, err)
	}
	return OK(c, nil)
}

// addToZip recursively adds a file or directory to the zip writer. counter takes
// the source bytes read; CreateZip passes nil, remote-to-remote being no transfer.
func addToZipDepth(zw *zip.Writer, client transfer.Client, remotePath, base string, counter prometheus.Counter, depth int, walks ...*diskQuota) error {
	if len(walks) == 0 {
		walks = []*diskQuota{{remaining: 100000, bytesLeft: maxZipSize}}
	}
	if err := walks[0].visit(); err != nil {
		return err
	}
	if depth > maxTreeDepth {
		return errTreeTooDeep
	}
	fi, err := client.Stat(remotePath)
	if err != nil {
		return err
	}
	if err := transfer.ValidateChildName(fi.Name, true); err != nil {
		return err
	}
	entryName := base + fi.Name
	if fi.IsDir {
		entryName += "/"
		entries, err := client.List(remotePath)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := transfer.ValidateChildName(e.Name, true); err != nil {
				return err
			}
		}
		for _, e := range entries {
			childPath := remotePath + "/" + e.Name
			if err := addToZipDepth(zw, client, childPath, entryName, counter, depth+1, walks...); err != nil {
				return err
			}
		}
		return nil
	}
	w, err := zw.Create(entryName)
	if err != nil {
		return err
	}
	r, err := client.Download(remotePath)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(w, io.LimitReader(metrics.CountingReader(&contextReader{ctx: walkContext(walks[0]), r: r}, counter), walks[0].bytesLeft+1))
	if n > walks[0].bytesLeft {
		copyErr = errArchiveTooLarge
	}
	// A short member would otherwise be zipped up and served as complete.
	walks[0].bytesLeft -= n
	closeErr := r.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return fmt.Errorf("%w: %w", transfer.ErrTransferIncomplete, err)
	}
	return nil
}

type archiveInput struct {
	r         io.Reader
	remaining int64
	ctx       context.Context
}

func (r *archiveInput) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.remaining <= 0 {
		return 0, errArchiveTooLarge
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func walkContext(q *diskQuota) context.Context {
	if q.ctx != nil {
		return q.ctx
	}
	return context.Background()
}

package api

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
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

// extractBudget caps the total DECOMPRESSED bytes an extraction may write to the
// remote. maxZipSize bounds only the compressed upload: 1 MB of bz2 expands to GBs.
type extractBudget struct{ remaining int64 }

func newExtractBudget() *extractBudget { return &extractBudget{remaining: maxZipSize} }

// wrap returns r limited to the budget still available, decrementing as it is
// consumed. Reading past the budget fails the whole extraction.
func (b *extractBudget) wrap(r io.Reader) io.Reader { return &budgetReader{b: b, r: r} }

type budgetReader struct {
	b *extractBudget
	r io.Reader
}

func (br *budgetReader) Read(p []byte) (int, error) {
	if br.b.remaining <= 0 {
		return 0, errArchiveTooLarge
	}
	if int64(len(p)) > br.b.remaining {
		p = p[:br.b.remaining]
	}
	n, err := br.r.Read(p)
	br.b.remaining -= int64(n)
	return n, err
}

// safeJoin joins destination and name, returning an error if the result escapes destination.
func safeJoin(destination, name string) (string, error) {
	outPath := path.Clean(path.Join(destination, name))
	cleanDest := path.Clean(destination)
	// Trailing slash so "/dir" cannot prefix-match "/dir2".
	if outPath != cleanDest && !strings.HasPrefix(outPath, cleanDest+"/") {
		return "", fmt.Errorf("archive entry %q escapes destination", name)
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
	if destination == "" {
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
		zr, err := zip.NewReader(f, fh.Size)
		if err != nil {
			return Fail(c, gftperrors.New(gftperrors.ErrArchiveFormat, "invalid zip archive"))
		}
		if err := extractZip(client, zr, destination, newExtractBudget()); err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
	case strings.HasSuffix(filename, ".tar.gz") || strings.HasSuffix(filename, ".tgz"):
		gr, err := gzip.NewReader(f)
		if err != nil {
			return Fail(c, gftperrors.New(gftperrors.ErrArchiveFormat, "invalid gzip archive"))
		}
		defer gr.Close()
		if err := extractTar(client, tar.NewReader(gr), destination, newExtractBudget()); err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
	case strings.HasSuffix(filename, ".tar.bz2"):
		if err := extractTar(client, tar.NewReader(bzip2.NewReader(f)), destination, newExtractBudget()); err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
	case strings.HasSuffix(filename, ".tar"):
		if err := extractTar(client, tar.NewReader(f), destination, newExtractBudget()); err != nil {
			return failClient(c, gftperrors.ErrOperationFailed, err)
		}
	default:
		return Fail(c, gftperrors.New(gftperrors.ErrArchiveFormat, "unsupported archive format"))
	}
	return OK(c, nil)
}

func extractZip(client transfer.Client, zr *zip.Reader, destination string, budget *extractBudget) error {
	for _, entry := range zr.File {
		outPath, err := safeJoin(destination, entry.Name)
		if err != nil {
			return err
		}
		if entry.FileInfo().IsDir() {
			if err := ensureDirAll(client, outPath); err != nil {
				return err
			}
			continue
		}
		if err := ensureDirAll(client, path.Dir(outPath)); err != nil {
			return err
		}
		rc, err := entry.Open()
		if err != nil {
			return err
		}
		err = client.Upload(outPath, budget.wrap(rc))
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTar(client transfer.Client, tr *tar.Reader, destination string, budget *extractBudget) error {
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		outPath, err := safeJoin(destination, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := ensureDirAll(client, outPath); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := ensureDirAll(client, path.Dir(outPath)); err != nil {
				return err
			}
			if err := client.Upload(outPath, budget.wrap(tr)); err != nil {
				return err
			}
		}
	}
	return nil
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
	if err := c.Bind(&req); err != nil || len(req.Paths) == 0 || req.Destination == "" {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "paths and destination are required"))
	}

	// Bounded before anything is written: the archive is spooled to the data
	// dir, so an unbounded input would fill the volume.
	var totalSize int64
	for _, p := range req.Paths {
		size, err := zipInputSize(client, p)
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
	tmp, err := os.CreateTemp(h.cfg.DataDir, "gftp-zip-*")
	if err != nil {
		return Fail(c, gftperrors.New(gftperrors.ErrInternal, "could not stage archive").WithCause(err))
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	zw := zip.NewWriter(tmp)
	for _, p := range req.Paths {
		if err := addToZip(zw, client, p, "", nil); err != nil {
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
func addToZip(zw *zip.Writer, client transfer.Client, remotePath, base string, counter prometheus.Counter) error {
	return addToZipDepth(zw, client, remotePath, base, counter, 0)
}

func addToZipDepth(zw *zip.Writer, client transfer.Client, remotePath, base string, counter prometheus.Counter, depth int) error {
	if depth > maxTreeDepth {
		return errTreeTooDeep
	}
	fi, err := client.Stat(remotePath)
	if err != nil {
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
			childPath := remotePath + "/" + e.Name
			if err := addToZipDepth(zw, client, childPath, entryName, counter, depth+1); err != nil {
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
	defer r.Close()
	_, err = io.Copy(w, metrics.CountingReader(r, counter))
	return err
}

package api

import (
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// maxUploadCheckPaths bounds how long one pre-flight holds the session's transfer
// lock. Beyond it the client uploads unchecked and the per-request guard decides.
const maxUploadCheckPaths = 10000

type uploadConflict struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	SuggestedName string `json:"suggestedName"`
	Size          int64  `json:"size"`
	IsDir         bool   `json:"isDir"`
	Modified      string `json:"modified"`
}

type uploadCheckResult struct {
	Conflicts []uploadConflict `json:"conflicts"`
}

// UploadCheck reports which destinations already exist, so conflicts are resolved
// before any transfer. It lists each parent once: on FTP a Stat is a whole LIST.
func (h *Handler) UploadCheck(c echo.Context) error {
	client, release, ok := lockedClient(c)
	if !ok {
		return Fail(c, gftperrors.New(gftperrors.ErrSessionNotFound, "no active connection"))
	}
	defer release()

	var req struct {
		Paths []string `json:"paths"`
	}
	if err := c.Bind(&req); err != nil || len(req.Paths) == 0 {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "paths are required"))
	}
	if len(req.Paths) > maxUploadCheckPaths {
		return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "too many paths"))
	}

	// Group by parent, preserving first-seen order so the response is stable.
	var order []string
	byDir := make(map[string][]string)
	for _, p := range req.Paths {
		if !strings.HasPrefix(p, "/") {
			return Fail(c, gftperrors.New(gftperrors.ErrBadRequest, "paths must be absolute"))
		}
		clean := path.Clean(p)
		dir := path.Dir(clean)
		if _, seen := byDir[dir]; !seen {
			order = append(order, dir)
		}
		byDir[dir] = append(byDir[dir], clean)
	}

	// Non-nil so the JSON carries an array rather than null: the SPA reads
	// conflicts.length unconditionally.
	result := uploadCheckResult{Conflicts: []uploadConflict{}}
	for _, dir := range order {
		entries, err := client.List(dir)
		if err != nil {
			if isConnLost(err) {
				return failClient(c, gftperrors.ErrListFailed, err)
			}
			// The directory does not exist yet (ensureDirAll will create it), so
			// nothing conflicts. A permission failure looks the same and reports "free".
			continue
		}
		existing := make(map[string]transfer.FileInfo, len(entries))
		taken := make(map[string]struct{}, len(entries)+len(byDir[dir]))
		for _, e := range entries {
			existing[e.Name] = e
			taken[e.Name] = struct{}{}
		}
		// Reserve every name this batch is about to write into the directory, so
		// a suggestion never lands on a sibling that is also queued.
		for _, p := range byDir[dir] {
			taken[path.Base(p)] = struct{}{}
		}
		for _, p := range byDir[dir] {
			name := path.Base(p)
			fi, clash := existing[name]
			if !clash {
				continue
			}
			suggested := uniqueName(name, taken)
			taken[suggested] = struct{}{}
			result.Conflicts = append(result.Conflicts, uploadConflict{
				Path:          p,
				Name:          name,
				SuggestedName: suggested,
				Size:          fi.Size,
				IsDir:         fi.IsDir,
				Modified:      time.Unix(fi.ModTime, 0).UTC().Format(time.RFC3339),
			})
		}
	}
	return OK(c, result)
}

// uniqueName returns name if free, else inserts " (1)", " (2)", … before the
// extension. Not the paste flow's "(copy)": an upload is a duplicate arrival.
func uniqueName(name string, taken map[string]struct{}) string {
	if _, clash := taken[name]; !clash {
		return name
	}
	base, ext := name, ""
	// A leading dot is the filename, not an extension: ".env" → ".env (1)".
	if dot := strings.LastIndex(name, "."); dot > 0 {
		base, ext = name[:dot], name[dot:]
	}
	for i := 1; ; i++ {
		candidate := base + " (" + strconv.Itoa(i) + ")" + ext
		if _, clash := taken[candidate]; !clash {
			return candidate
		}
	}
}

package sftp

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	psftp "github.com/pkg/sftp"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// Archive writes require stable parent paths because SFTP v3 has no openat.
type archiveTarget struct {
	c   *Client
	uid uint32
}

// SetArchiveAccount binds the operator's POSIX identity to the authenticated host.
func (c *Client) SetArchiveAccount(user string, accounts map[string]map[string]uint32) {
	c.archiveUID = nil
	if uid, ok := accounts[c.hostFingerprint][user]; ok {
		c.archiveUID = &uid
	}
}

func (c *Client) ArchiveTarget() (transfer.ArchiveTarget, func(), error) {
	if c.archiveUID == nil {
		return nil, nil, fmt.Errorf("%w: unverified POSIX account", transfer.ErrPermissionDenied)
	}
	defer c.boundIO()()
	scoped, closeChannel, err := c.archiveChannel()
	if err != nil {
		return nil, nil, err
	}
	return &archiveTarget{c: scoped, uid: *c.archiveUID}, closeChannel, nil
}

func (a *archiveTarget) EnsureDir(dir string) error {
	defer a.c.boundIO()()
	current := "/"
	for _, part := range strings.Split(path.Clean(dir), "/") {
		current = path.Join(current, part)
		info, err := a.c.sftp.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if err := a.c.sftp.Mkdir(current); err != nil {
				return wrapErr(err)
			}
			info, err = a.c.sftp.Lstat(current)
		}
		if err != nil {
			return wrapErr(err)
		}
		if !a.trustedDirectory(info) {
			return fmt.Errorf("%w: unsafe archive directory", transfer.ErrPermissionDenied)
		}
	}
	return nil
}

func (a *archiveTarget) trustedDirectory(info fs.FileInfo) bool {
	owner, ok := info.Sys().(*psftp.FileStat)
	return ok && info.IsDir() && (owner.UID == 0 || owner.UID == a.uid) && info.Mode().Perm()&0o022 == 0
}

func (a *archiveTarget) Upload(p string, r io.Reader) error {
	// Exclusive creation also protects dangling symlinks and existing hard links.
	done := a.c.boundIO()
	f, err := a.c.sftp.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	done()
	if err != nil {
		return wrapErr(err)
	}
	return a.c.writeFile(f, r)
}

package sftp

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// Client wraps pkg/sftp and implements transfer.Client.
type Client struct {
	ssh  *ssh.Client
	sftp *sftp.Client
}

// Dial connects via SSH and opens an SFTP subsystem.
// Phase 3 uses InsecureIgnoreHostKey — Phase 4 will add key verification.
func Dial(addr, user, pass string) (*Client, error) {
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // Phase 4 will fix this
		Timeout:         10 * time.Second,
	}
	sshConn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		msg := err.Error()
		if isAuthErr(msg) {
			return nil, fmt.Errorf("%w: %v", transfer.ErrAuthFailed, err)
		}
		return nil, fmt.Errorf("%w: %v", transfer.ErrConnectionFailed, err)
	}
	sftpClient, err := sftp.NewClient(sshConn)
	if err != nil {
		sshConn.Close()
		return nil, fmt.Errorf("%w: %v", transfer.ErrConnectionFailed, err)
	}
	return &Client{ssh: sshConn, sftp: sftpClient}, nil
}

func (c *Client) WorkingDir() (string, error) {
	return c.sftp.Getwd()
}

func (c *Client) List(dir string) ([]transfer.FileInfo, error) {
	entries, err := c.sftp.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]transfer.FileInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, infoFromFS(e))
	}
	return out, nil
}

func (c *Client) Stat(p string) (transfer.FileInfo, error) {
	fi, err := c.sftp.Stat(p)
	if err != nil {
		return transfer.FileInfo{}, err
	}
	return infoFromFS(fi), nil
}

func (c *Client) MakeDir(p string) error {
	return c.sftp.MkdirAll(p)
}

func (c *Client) Delete(p string) error {
	fi, err := c.sftp.Stat(p)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return c.sftp.RemoveAll(p)
	}
	return c.sftp.Remove(p)
}

func (c *Client) Rename(src, dst string) error {
	return c.sftp.Rename(src, dst)
}

func (c *Client) Chmod(p string, mode uint32) error {
	err := c.sftp.Chmod(p, fs.FileMode(mode))
	if err != nil && errors.Is(err, sftp.ErrSSHFxOpUnsupported) {
		return transfer.ErrPermissionsNotSupported
	}
	return err
}

func (c *Client) Download(p string) (io.ReadCloser, error) {
	return c.sftp.Open(p)
}

func (c *Client) Upload(p string, r io.Reader) error {
	f, err := c.sftp.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (c *Client) Ping() error {
	_, err := c.sftp.Getwd()
	return err
}

func (c *Client) Close() error {
	sftpErr := c.sftp.Close()
	sshErr := c.ssh.Close()
	if sftpErr != nil {
		return sftpErr
	}
	return sshErr
}

var _ transfer.Client = (*Client)(nil)

func infoFromFS(fi fs.FileInfo) transfer.FileInfo {
	return transfer.FileInfo{
		Name:        fi.Name(),
		Size:        fi.Size(),
		IsDir:       fi.IsDir(),
		ModTime:     fi.ModTime().Unix(),
		Permissions: fi.Mode().String(),
	}
}

func isAuthErr(msg string) bool {
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "auth fail")
}

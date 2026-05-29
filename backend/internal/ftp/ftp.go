package ftp

import (
	"fmt"
	"io"
	"path"
	"time"

	jftp "github.com/jlaffaye/ftp"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

// Client wraps jlaffaye/ftp and implements transfer.Client.
type Client struct {
	conn *jftp.ServerConn
}

// Dial connects and authenticates. passive controls passive/active mode.
func Dial(addr, user, pass string, passive bool) (*Client, error) {
	conn, err := jftp.Dial(addr, jftp.DialWithTimeout(10*time.Second))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", transfer.ErrConnectionFailed, err)
	}
	if err := conn.Login(user, pass); err != nil {
		conn.Quit()
		return nil, fmt.Errorf("%w: %v", transfer.ErrAuthFailed, err)
	}
	return &Client{conn: conn}, nil
}

func (c *Client) WorkingDir() (string, error) {
	return c.conn.CurrentDir()
}

func (c *Client) List(dir string) ([]transfer.FileInfo, error) {
	entries, err := c.conn.List(dir)
	if err != nil {
		return nil, err
	}
	out := make([]transfer.FileInfo, 0, len(entries))
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		out = append(out, transfer.FileInfo{
			Name:    e.Name,
			Size:    int64(e.Size),
			IsDir:   e.Type == jftp.EntryTypeFolder,
			ModTime: e.Time.Unix(),
		})
	}
	return out, nil
}

func (c *Client) Stat(p string) (transfer.FileInfo, error) {
	if p == "/" {
		return transfer.FileInfo{Name: "/", IsDir: true}, nil
	}
	parent := path.Dir(p)
	name := path.Base(p)
	entries, err := c.conn.List(parent)
	if err != nil {
		return transfer.FileInfo{}, err
	}
	for _, e := range entries {
		if e.Name == name {
			return transfer.FileInfo{
				Name:    e.Name,
				Size:    int64(e.Size),
				IsDir:   e.Type == jftp.EntryTypeFolder,
				ModTime: e.Time.Unix(),
			}, nil
		}
	}
	return transfer.FileInfo{}, fmt.Errorf("stat %s: not found", p)
}

func (c *Client) MakeDir(p string) error {
	return c.conn.MakeDir(p)
}

func (c *Client) Delete(p string) error {
	err := c.conn.Delete(p)
	if err != nil {
		return c.conn.RemoveDirRecur(p)
	}
	return nil
}

func (c *Client) Rename(src, dst string) error {
	return c.conn.Rename(src, dst)
}

func (c *Client) Chmod(p string, mode uint32) error {
	// jlaffaye/ftp does not support SITE CHMOD, and FTP chmod is not
	// universally supported across servers anyway.
	return transfer.ErrPermissionsNotSupported
}

func (c *Client) Download(p string) (io.ReadCloser, error) {
	return c.conn.Retr(p)
}

func (c *Client) Upload(p string, r io.Reader) error {
	return c.conn.Stor(p, r)
}

func (c *Client) Close() error {
	return c.conn.Quit()
}

// Ensure *Client implements transfer.Client at compile time.
var _ transfer.Client = (*Client)(nil)

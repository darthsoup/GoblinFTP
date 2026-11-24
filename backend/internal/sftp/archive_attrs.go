package sftp

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"

	psftp "github.com/pkg/sftp"
)

// pkg/sftp decodes an omitted UID as zero. On the extraction channel, require
// both ownership and permissions on every SFTP v3 ATTRS reply before decoding.
type archiveReplies struct {
	r         *bufio.Reader
	remaining int64
}

func (a *archiveReplies) Read(p []byte) (int, error) {
	if a.remaining == 0 {
		header, err := a.r.Peek(5)
		if err != nil {
			return 0, err
		}
		size := binary.BigEndian.Uint32(header[:4])
		if size == 0 {
			return 0, errors.New("empty SFTP packet")
		}
		if header[4] == 105 {
			if size < 9 {
				return 0, errors.New("truncated SFTP attributes")
			}
			attrs, err := a.r.Peek(13)
			if err != nil {
				return 0, err
			}
			const uidGIDAndPermissions = 0x06
			if binary.BigEndian.Uint32(attrs[9:13])&uidGIDAndPermissions != uidGIDAndPermissions {
				return 0, errors.New("SFTP owner or permissions unavailable")
			}
		}
		a.remaining = int64(size) + 4
	}
	if int64(len(p)) > a.remaining {
		p = p[:a.remaining]
	}
	n, err := a.r.Read(p)
	a.remaining -= int64(n)
	return n, err
}

func (c *Client) archiveChannel() (*Client, func(), error) {
	session, err := c.ssh.NewSession()
	if err != nil {
		return nil, nil, err
	}
	closeSession := func() { _ = session.Close() }
	reader, err := session.StdoutPipe()
	if err != nil {
		closeSession()
		return nil, nil, err
	}
	writer, err := session.StdinPipe()
	if err != nil {
		closeSession()
		return nil, nil, err
	}
	session.Stderr = io.Discard
	if err := session.RequestSubsystem("sftp"); err != nil {
		closeSession()
		return nil, nil, err
	}
	sub, err := psftp.NewClientPipe(&archiveReplies{r: bufio.NewReader(reader)}, writer)
	if err != nil {
		closeSession()
		return nil, nil, err
	}
	scoped := *c
	scoped.sftp = sub
	return &scoped, func() { defer c.boundIO()(); closeSession(); _ = sub.Close() }, nil
}

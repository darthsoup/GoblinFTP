// backend/internal/api/dial.go
package api

import (
	"fmt"

	ftpadapter "github.com/darthsoup/goblinftp/internal/ftp"
	sftpadapter "github.com/darthsoup/goblinftp/internal/sftp"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// defaultDial routes to the FTP or SFTP adapter based on protocol.
func defaultDial(protocol, addr, user, pass string, passive bool) (transfer.Client, error) {
	switch protocol {
	case "ftp":
		return ftpadapter.Dial(addr, user, pass, passive)
	case "sftp":
		return sftpadapter.Dial(addr, user, pass)
	default:
		return nil, fmt.Errorf("%w: unknown protocol %q", transfer.ErrConnectionFailed, protocol)
	}
}

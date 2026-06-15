// backend/internal/api/dial.go
package api

import (
	"crypto/tls"
	"fmt"
	"path/filepath"

	"github.com/darthsoup/goblinftp/internal/config"
	ftpadapter "github.com/darthsoup/goblinftp/internal/ftp"
	sftpadapter "github.com/darthsoup/goblinftp/internal/sftp"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

// newDefaultDial builds the production dialer, closing over cfg so it can apply
// the FTPS TLS policy and locate the SFTP known_hosts file under the data dir.
func newDefaultDial(cfg *config.Config) DialFunc {
	knownHostsPath := filepath.Join(cfg.DataDir, "known_hosts")
	return func(req DialRequest) (transfer.Client, *HostKeyPrompt, error) {
		switch req.Protocol {
		case "ftp":
			c, err := ftpadapter.Dial(req.Addr, req.User, req.Pass, req.Passive, nil)
			if err != nil {
				return nil, nil, err
			}
			return c, nil, nil
		case "ftps":
			tlsCfg := &tls.Config{
				ServerName:         req.Host,
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: cfg.Settings.Connection.FTPTLSInsecureSkipVerify, //nolint:gosec // G402: admin opt-in for self-signed/internal FTPS servers
			}
			c, err := ftpadapter.Dial(req.Addr, req.User, req.Pass, req.Passive, tlsCfg)
			if err != nil {
				return nil, nil, err
			}
			return c, nil, nil
		case "sftp":
			c, prompt, err := sftpadapter.Dial(req.Addr, req.User, req.Pass, req.AcceptHostKey, knownHostsPath)
			if prompt != nil {
				return nil, &HostKeyPrompt{Host: req.Host, Fingerprint: prompt.Fingerprint, KeyType: prompt.KeyType}, nil
			}
			if err != nil {
				return nil, nil, err
			}
			return c, nil, nil
		default:
			return nil, nil, fmt.Errorf("%w: unknown protocol %q", transfer.ErrConnectionFailed, req.Protocol)
		}
	}
}

package api_test

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/ftp"
	"github.com/darthsoup/goblinftp/internal/sftp"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestDirectoryTreeIntegration(t *testing.T) {
	for _, protocol := range []string{"ftp", "ftps", "sftp"} {
		t.Run(protocol, func(t *testing.T) {
			prefix := "GFTP_TEST_" + strings.ToUpper(protocol)
			host := os.Getenv(prefix + "_HOST")
			if host == "" {
				t.Skip("set " + prefix + "_HOST to run directory integration tests")
			}
			value := func(key, fallback string) string {
				if v := os.Getenv(prefix + key); v != "" {
					return v
				}
				return fallback
			}
			user, pass := value("_USER", "ftpuser"), value("_PASS", "ftppass")
			var client transfer.Client
			if protocol == "sftp" {
				knownHosts := filepath.Join(t.TempDir(), "known_hosts")
				_, prompt, err := sftp.Dial(host, user, pass, "", knownHosts)
				require.NoError(t, err)
				require.NotNil(t, prompt)
				c, _, err := sftp.Dial(host, user, pass, prompt.Fingerprint, knownHosts)
				require.NoError(t, err)
				client = c
			} else {
				var tlsConfig *tls.Config
				if protocol == "ftps" {
					// The local FTPS fixture uses a self-signed certificate.
					tlsConfig = &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
				}
				c, err := ftp.Dial(host, user, pass, true, tlsConfig)
				require.NoError(t, err)
				client = c
			}
			defer func() { _ = client.Close() }()
			base, err := client.WorkingDir()
			require.NoError(t, err)
			if protocol == "sftp" {
				base = value("_DIR", "/upload")
			}
			root := path.Join(base, fmt.Sprintf("gftp-tree-test-%d", time.Now().UnixNano()))
			require.NoError(t, client.MakeDir(root))
			private := path.Join(root, "private")
			defer func() {
				if client.SupportsChmod() {
					_ = client.Chmod(private, 0o700)
				}
				_ = client.Delete(path.Join(root, ".hidden"))
				require.NoError(t, client.Delete(root))
			}()
			for _, dir := range []string{"nested", "nested/empty", ".hidden", "private"} {
				require.NoError(t, client.MakeDir(path.Join(root, dir)))
			}
			require.NoError(t, client.Upload(path.Join(root, "file.txt"), strings.NewReader("tree fixture")))
			app, store, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(client)))
			t.Cleanup(store.Close)
			session := connectAndGetSession(t, app)
			list := func(dir string, status int) []struct {
				Name  string `json:"name"`
				IsDir bool   `json:"isDir"`
			} {
				req := httptest.NewRequest(http.MethodGet, "/api/files?path="+url.QueryEscape(dir), nil)
				addSession(req, session)
				rec := httptest.NewRecorder()
				app.ServeHTTP(rec, req)
				require.Equal(t, status, rec.Code, rec.Body.String())
				var response struct {
					Data []struct {
						Name  string `json:"name"`
						IsDir bool   `json:"isDir"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
				return response.Data
			}
			list("/", http.StatusOK)
			entries := list(root, http.StatusOK)
			types := make(map[string]bool)
			for _, entry := range entries {
				types[entry.Name] = entry.IsDir
			}
			for name, isDir := range map[string]bool{"nested": true, "private": true, "file.txt": false} {
				require.Contains(t, types, name)
				require.Equal(t, isDir, types[name])
			}
			// FTP servers may omit dotfolders from LIST even when directly accessible.
			require.Empty(t, list(path.Join(root, ".hidden"), http.StatusOK))
			require.Len(t, list(path.Join(root, "nested"), http.StatusOK), 1)
			require.Empty(t, list(path.Join(root, "nested/empty"), http.StatusOK))
			if client.SupportsChmod() {
				require.NoError(t, client.Chmod(private, 0))
				list(private, http.StatusForbidden)
				list(path.Join(root, "nested"), http.StatusOK)
			}
		})
	}
}

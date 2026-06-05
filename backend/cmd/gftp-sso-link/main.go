// Command gftp-sso-link generates one-time SSO login links for GoblinFTP.
//
// It reuses internal/sso, so tokens are always compatible with the server.
// The shared secret must match the GFTP_SSO_SECRET the server runs with.
// See examples/sso/README.md for the token format and standalone
// implementations in other languages.
//
// Usage:
//
//	GFTP_SSO_SECRET=change-me go run ./cmd/gftp-sso-link \
//	  -host ftp.example.com -username alice -password s3cret \
//	  -base-url https://files.example.com
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/darthsoup/goblinftp/internal/sso"
)

func main() {
	protocol := flag.String("protocol", "ftp", "protocol: ftp or sftp")
	host := flag.String("host", "", "FTP/SFTP server host (required)")
	port := flag.Int("port", 0, "server port (default: 21 for ftp, 22 for sftp)")
	username := flag.String("username", "", "login username (required)")
	password := flag.String("password", "", "login password (or set GFTP_SSO_PASSWORD to keep it out of shell history)")
	dir := flag.String("dir", "", "initial directory hint (optional)")
	lang := flag.String("lang", "", "UI language hint, e.g. en or de (optional)")
	ttl := flag.Duration("ttl", 5*time.Minute, "token validity window")
	baseURL := flag.String("base-url", "http://localhost:8080", "public URL of the GoblinFTP instance")
	secret := flag.String("secret", "", "shared SSO secret (default: $GFTP_SSO_SECRET)")
	flag.Parse()

	if *secret == "" {
		*secret = os.Getenv("GFTP_SSO_SECRET")
	}
	if *password == "" {
		*password = os.Getenv("GFTP_SSO_PASSWORD")
	}

	fail := func(msg string) {
		fmt.Fprintf(os.Stderr, "error: %s\n\n", msg)
		flag.Usage()
		os.Exit(2)
	}
	if *secret == "" {
		fail("missing -secret (or set GFTP_SSO_SECRET)")
	}
	if *host == "" {
		fail("missing -host")
	}
	if *username == "" {
		fail("missing -username")
	}
	if *protocol != "ftp" && *protocol != "sftp" {
		fail("-protocol must be ftp or sftp")
	}
	if *port == 0 {
		if *protocol == "sftp" {
			*port = 22
		} else {
			*port = 21
		}
	}

	token, err := sso.Encrypt(&sso.Payload{
		Type:             *protocol,
		Host:             *host,
		Port:             *port,
		Username:         *username,
		Password:         *password,
		InitialDirectory: *dir,
		Language:         *lang,
		Exp:              time.Now().Add(*ttl).Unix(),
	}, []byte(*secret))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// Token is base64url (RFC 4648 §5, no padding) — already query-safe.
	fmt.Printf("%s/?sso=%s\n", strings.TrimRight(*baseURL, "/"), token)
}

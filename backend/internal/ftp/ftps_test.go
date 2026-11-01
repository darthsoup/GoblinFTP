package ftp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFTPSProtectsEveryDataConnection(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	serverTLS := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	done := make(chan error, 1)
	go func() { done <- serveProtectedFTP(ln, serverTLS) }()
	client, err := Dial(ln.Addr().String(), "u", "p", true, &tls.Config{RootCAs: roots, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
	require.NoError(t, client.Upload("/file.txt", strings.NewReader("private upload")))
	require.NoError(t, client.Upload("/empty.txt", strings.NewReader("")))
	reader, err := client.Download("/file.txt")
	require.NoError(t, err)
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, "private download", string(content))
	files, err := client.List("/")
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "file.txt", files[0].Name)
	require.NoError(t, client.Close())
	select {
	case err = <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("FTPS server did not complete")
	}
}

func serveProtectedFTP(ln net.Listener, cfg *tls.Config) error {
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	tp := textproto.NewConn(conn)
	if err = tp.PrintfLine("220 ready"); err != nil {
		return err
	}
	var data net.Listener
	defer func() {
		if data != nil {
			_ = data.Close()
		}
	}()
	for {
		line, readErr := tp.ReadLine()
		if readErr != nil {
			return readErr
		}
		command, argument, _ := strings.Cut(line, " ")
		switch command {
		case "AUTH":
			if err = tp.PrintfLine("234 proceed"); err != nil {
				return err
			}
			secure := tls.Server(conn, cfg)
			if err = secure.Handshake(); err != nil {
				return err
			}
			tp = textproto.NewConn(secure)
		case "USER":
			err = tp.PrintfLine("331 password")
		case "PASS":
			err = tp.PrintfLine("230 logged in")
		case "FEAT":
			err = tp.PrintfLine("500 unsupported")
		case "EPSV":
			data, err = net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return err
			}
			err = tp.PrintfLine("229 Entering Extended Passive Mode (|||%d|)", data.Addr().(*net.TCPAddr).Port)
		case "STOR", "RETR", "LIST":
			if data == nil {
				return fmt.Errorf("missing passive listener")
			}
			if err = tp.PrintfLine("150 opening data"); err != nil {
				return err
			}
			raw, acceptErr := data.Accept()
			_ = data.Close()
			data = nil
			if acceptErr != nil {
				return acceptErr
			}
			_ = raw.SetDeadline(time.Now().Add(3 * time.Second))
			secure := tls.Server(raw, cfg)
			if err = secure.Handshake(); err != nil {
				_ = raw.Close()
				return fmt.Errorf("%s data TLS: %w", command, err)
			}
			if !secure.ConnectionState().DidResume {
				return fmt.Errorf("%s data connection did not reuse the TLS session", command)
			}
			switch command {
			case "STOR":
				var body []byte
				body, err = io.ReadAll(secure)
				expected := "private upload"
				if argument == "/empty.txt" {
					expected = ""
				}
				if err == nil && string(body) != expected {
					err = fmt.Errorf("unexpected upload %q", body)
				}
			case "RETR":
				_, err = io.WriteString(secure, "private download")
			case "LIST":
				_, err = io.WriteString(secure, "-rw-r--r-- 1 user group 16 Oct 19 12:00 file.txt\r\n")
			}
			_ = secure.Close()
			if err != nil {
				return err
			}
			err = tp.PrintfLine("226 complete")
		case "QUIT":
			return tp.PrintfLine("221 bye")
		default:
			err = tp.PrintfLine("200 ok")
		}
		if err != nil {
			return err
		}
	}
}

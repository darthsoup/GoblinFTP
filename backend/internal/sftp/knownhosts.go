package sftp

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKeyPrompt describes an unverified SSH host key that the user must confirm
// before the connection can proceed (trust-on-first-use).
type HostKeyPrompt struct {
	Fingerprint string // SHA256:…
	KeyType     string // e.g. "ssh-ed25519"
}

// knownHostsMu serializes reads and appends of the known_hosts file across
// concurrent dials. The file is read fresh on every dial, so a key trusted in
// one connection is honored by the next.
var knownHostsMu sync.Mutex

// errHostKeyHalt aborts the SSH handshake before authentication once the
// callback has decided the key is unknown (captured for a prompt) or mismatched
// — so the password is never sent to an unverified host.
var errHostKeyHalt = errors.New("host key not verified")

// hostKeyResult is populated as a side effect of the host-key callback so Dial
// can decide what to return after ssh.Dial aborts.
type hostKeyResult struct {
	prompt   *HostKeyPrompt // host is unknown and not yet accepted
	mismatch bool           // a different key is already pinned (possible MITM)
}

// buildHostKeyCallback verifies the server key against knownHostsPath with
// trust-on-first-use semantics:
//   - known + match            → accept
//   - known + different key    → res.mismatch, halt (possible MITM)
//   - unknown + acceptFP match → pin to known_hosts, accept
//   - unknown otherwise        → res.prompt, halt (needs confirmation)
func buildHostKeyCallback(addr, knownHostsPath, acceptFingerprint string, res *hostKeyResult) (ssh.HostKeyCallback, error) {
	verify, err := loadKnownHosts(knownHostsPath)
	if err != nil {
		return nil, err
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := verify(hostname, remote, key)
		if err == nil {
			return nil // pinned key matches
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err // revoked or malformed entry → reject
		}
		if len(keyErr.Want) > 0 {
			res.mismatch = true // a different key is pinned → possible MITM
			return errHostKeyHalt
		}
		// Unknown host.
		fp := ssh.FingerprintSHA256(key)
		if acceptFingerprint != "" && acceptFingerprint == fp {
			if err := appendKnownHost(knownHostsPath, addr, key); err != nil {
				return err
			}
			return nil // trusted now → proceed to auth
		}
		res.prompt = &HostKeyPrompt{Fingerprint: fp, KeyType: key.Type()}
		return errHostKeyHalt
	}, nil
}

// loadKnownHosts ensures the file exists and parses it into a callback, holding
// the lock so a concurrent append can't be observed mid-write.
func loadKnownHosts(path string) (ssh.HostKeyCallback, error) {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return knownhosts.New(path)
}

// appendKnownHost pins key for addr by appending an OpenSSH known_hosts line.
func appendKnownHost(path, addr string, key ssh.PublicKey) error {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()
	line := knownhosts.Line([]string{knownhosts.Normalize(addr)}, key)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

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

// HostKeyPrompt describes an SSH host key the user must confirm before connecting:
// unverified (trust-on-first-use), or, when Changed is set, differing from the pin.
type HostKeyPrompt struct {
	Fingerprint    string // SHA256:…
	KeyType        string // e.g. "ssh-ed25519"
	Changed        bool   // a different key is pinned (server reinstalled, or MITM)
	OldFingerprint string // previously pinned key's fingerprint (set when Changed)
}

// knownHostsMu serializes reads and appends of known_hosts across concurrent dials.
// The file is read fresh per dial, so a key trusted in one connection binds the next.
var knownHostsMu sync.Mutex

// errHostKeyHalt aborts the SSH handshake before authentication once the callback
// finds the key unknown or mismatched, so no password reaches an unverified host.
var errHostKeyHalt = errors.New("host key not verified")

// hostKeyResult is populated as a side effect of the host-key callback so Dial
// can decide what to return after ssh.Dial aborts.
type hostKeyResult struct {
	fingerprint string
	prompt      *HostKeyPrompt // key is unknown or changed, not yet accepted
	mismatch    bool
	revoked     bool  // the key is explicitly @revoked: never offer to pin it
	writeErr    error // known_hosts could not be read or written
}

// buildHostKeyCallback verifies the server key against knownHostsPath with trust-on-first-use:
// an accepted first-use fingerprint pins a new host; changed keys fail closed.
func buildHostKeyCallback(addr, knownHostsPath, acceptFingerprint string, res *hostKeyResult) (ssh.HostKeyCallback, error) {
	_, err := loadKnownHosts(knownHostsPath)
	if err != nil {
		return nil, err
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()
		verify, loadErr := loadKnownHostsLocked(knownHostsPath)
		if loadErr != nil {
			res.writeErr = loadErr
			return errHostKeyHalt
		}
		res.fingerprint = ssh.FingerprintSHA256(key)
		err := verify(hostname, remote, key)
		if err == nil {
			return nil // pinned key matches
		}
		// An explicitly revoked key is never re-pinnable, so it must be caught
		// before the KeyError branch offers the user a confirmation prompt.
		var revokedErr *knownhosts.RevokedError
		if errors.As(err, &revokedErr) {
			res.revoked = true
			return errHostKeyHalt
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err // malformed entry → reject
		}
		fp := res.fingerprint
		// A shared host pin can only be rotated by the operator.
		if len(keyErr.Want) != 0 {
			res.mismatch = true
			return errHostKeyHalt
		}
		// Unknown host.
		if acceptFingerprint != "" && acceptFingerprint == fp {
			if err := appendKnownHostLocked(knownHostsPath, addr, key); err != nil {
				res.writeErr = err
				return errHostKeyHalt
			}
			return reverifyPin(knownHostsPath, hostname, remote, key, res)
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
	return loadKnownHostsLocked(path)
}

func loadKnownHostsLocked(path string) (ssh.HostKeyCallback, error) {
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
	return appendKnownHostLocked(path, addr, key)
}

func appendKnownHostLocked(path, addr string, key ssh.PublicKey) error {
	line := knownhosts.Line([]string{knownhosts.Normalize(addr)}, key)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

func reverifyPin(path, hostname string, remote net.Addr, key ssh.PublicKey, res *hostKeyResult) error {
	verify, err := loadKnownHostsLocked(path)
	if err == nil {
		err = verify(hostname, remote, key)
	}
	if err != nil {
		res.writeErr = err
		return errHostKeyHalt
	}
	return nil
}

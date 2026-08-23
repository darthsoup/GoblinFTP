package sftp

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	prompt *HostKeyPrompt // key is unknown or changed, not yet accepted
}

// buildHostKeyCallback verifies the server key against knownHostsPath with trust-on-first-use:
// a pinned match accepts, an acceptFingerprint match pins or re-pins, anything else halts with res.prompt.
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
		fp := ssh.FingerprintSHA256(key)
		if len(keyErr.Want) > 0 {
			// A different key is pinned (server reinstalled, or MITM). Replacing it
			// needs explicit confirmation against the new key's fingerprint.
			if acceptFingerprint != "" && acceptFingerprint == fp {
				if err := replaceKnownHost(knownHostsPath, addr, key); err != nil {
					return err
				}
				return nil // re-trusted → proceed to auth
			}
			res.prompt = &HostKeyPrompt{
				Fingerprint:    fp,
				KeyType:        key.Type(),
				Changed:        true,
				OldFingerprint: ssh.FingerprintSHA256(keyErr.Want[0].Key),
			}
			return errHostKeyHalt
		}
		// Unknown host.
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

// replaceKnownHost re-pins addr to key: plain entries for addr are dropped, the new line appended.
// Hashed (|1|…) and marker (@…) lines are unmatchable textually and stay; the app writes only plain ones.
func replaceKnownHost(path, addr string, key ssh.PublicKey) error {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	norm := knownhosts.Normalize(addr)
	var out []string
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(line, "\n")
		fields := strings.Fields(line)
		if len(fields) >= 3 && !strings.HasPrefix(fields[0], "@") && !strings.HasPrefix(fields[0], "|") {
			hosts := slices.DeleteFunc(strings.Split(fields[0], ","), func(h string) bool { return h == norm })
			if len(hosts) == 0 {
				continue
			}
			fields[0] = strings.Join(hosts, ",")
			line = strings.Join(fields, " ")
		}
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	out = append(out, knownhosts.Line([]string{norm}, key))
	return writeFileAtomic(path, []byte(strings.Join(out, "\n")+"\n"))
}

// writeFileAtomic replaces path via a same-filesystem temp file plus rename(2). os.WriteFile
// truncates first, so a crash mid-write left an empty known_hosts: silent first-use trust, no error.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once the rename succeeded

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	// Durability before the rename: otherwise a power loss can leave the
	// renamed entry pointing at unwritten data.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

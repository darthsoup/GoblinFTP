package sftp

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // G505: known_hosts hashing is defined as HMAC-SHA1 by OpenSSH
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/darthsoup/goblinftp/internal/transfer"
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
	prompt   *HostKeyPrompt // key is unknown or changed, not yet accepted
	revoked  bool           // the key is explicitly @revoked: never offer to pin it
	writeErr error          // known_hosts could not be read or written
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
		fp := ssh.FingerprintSHA256(key)
		// Only a pin of the SAME algorithm means the host key changed. A server
		// that added an algorithm is an unknown key, not a man-in-the-middle.
		sameType := sameAlgorithm(keyErr.Want, key.Type())
		if len(sameType) > 0 {
			// A different key is pinned (server reinstalled, or MITM). Replacing it
			// needs explicit confirmation against the new key's fingerprint.
			if acceptFingerprint != "" && acceptFingerprint == fp {
				if err := replaceKnownHost(knownHostsPath, addr, key); err != nil {
					res.writeErr = err
					return errHostKeyHalt
				}
				return nil // re-trusted → proceed to auth
			}
			res.prompt = &HostKeyPrompt{
				Fingerprint:    fp,
				KeyType:        key.Type(),
				Changed:        true,
				OldFingerprint: ssh.FingerprintSHA256(sameType[0].Key),
			}
			return errHostKeyHalt
		}
		// Unknown host.
		if acceptFingerprint != "" && acceptFingerprint == fp {
			if err := appendKnownHost(knownHostsPath, addr, key); err != nil {
				res.writeErr = err
				return errHostKeyHalt
			}
			return nil // trusted now → proceed to auth
		}
		res.prompt = &HostKeyPrompt{Fingerprint: fp, KeyType: key.Type()}
		return errHostKeyHalt
	}, nil
}

// sameAlgorithm keeps only the pinned keys using keyType, so an added host-key
// algorithm reads as a first-use prompt instead of a MITM warning.
func sameAlgorithm(want []knownhosts.KnownKey, keyType string) []knownhosts.KnownKey {
	out := make([]knownhosts.KnownKey, 0, len(want))
	for _, w := range want {
		if w.Key.Type() == keyType {
			out = append(out, w)
		}
	}
	return out
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

// replaceKnownHost re-pins addr to key. Plain and hashed (|1|salt|hash) entries
// for addr are both dropped: leaving a hashed one in place made re-pinning a
// silent no-op that kept trusting the old key.
func replaceKnownHost(path, addr string, key ssh.PublicKey) error {
	knownHostsMu.Lock()
	defer knownHostsMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	norm := knownhosts.Normalize(addr)
	var out []string
	stale := 0
	for line := range strings.Lines(string(data)) {
		line = strings.TrimRight(line, "\n")
		fields := strings.Fields(line)
		if len(fields) < 3 {
			if strings.TrimSpace(line) != "" {
				out = append(out, line)
			}
			continue
		}
		switch {
		case strings.HasPrefix(fields[0], "@"):
			// @cert-authority and @revoked are deliberate operator statements and
			// are left alone, but a surviving one for this host is not safe to
			// ignore: the caller is told rather than silently trusting the old key.
			if markerCoversHost(fields, norm) {
				stale++
			}
			out = append(out, line)
		case strings.HasPrefix(fields[0], "|"):
			if hashedHostMatches(fields[0], norm) {
				continue
			}
			out = append(out, line)
		default:
			hosts := slices.DeleteFunc(strings.Split(fields[0], ","), func(h string) bool { return h == norm })
			if len(hosts) == 0 {
				continue
			}
			fields[0] = strings.Join(hosts, ",")
			out = append(out, strings.Join(fields, " "))
		}
	}
	if stale > 0 {
		return fmt.Errorf("%w: known_hosts still carries a marker entry for %s, clean it up by hand",
			transfer.ErrHostKeyStoreUnavailable, norm)
	}
	out = append(out, knownhosts.Line([]string{norm}, key))
	return writeFileAtomic(path, []byte(strings.Join(out, "\n")+"\n"))
}

// hashedHostMatches recomputes the |1|salt|hash form with the line's OWN salt.
// knownhosts.HashHostname generates a fresh salt each call, so it can never be
// used to match an existing entry.
func hashedHostMatches(field, host string) bool {
	parts := strings.Split(field, "|")
	// "" | "1" | salt | hash
	if len(parts) != 4 || parts[1] != "1" {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(host))
	return hmac.Equal(mac.Sum(nil), want)
}

// markerCoversHost reports whether an @-marker line names host.
func markerCoversHost(fields []string, host string) bool {
	if len(fields) < 2 {
		return false
	}
	return slices.Contains(strings.Split(fields[1], ","), host)
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

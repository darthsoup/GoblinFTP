package transfer

import (
	"errors"
	"io"
)

// FileInfo represents a single remote filesystem entry.
type FileInfo struct {
	Name        string
	Size        int64
	IsDir       bool
	ModTime     int64  // Unix timestamp
	Permissions string // e.g. "drwxr-xr-x"
}

// Client is the unified interface that both FTP and SFTP adapters implement.
// All methods that accept a path expect an absolute path on the remote server.
type Client interface {
	// WorkingDir returns the current working directory.
	WorkingDir() (string, error)
	// List returns the contents of the given directory.
	List(path string) ([]FileInfo, error)
	// Stat returns info for a single path. On FTP, this lists the parent dir
	// and finds the entry by name.
	Stat(path string) (FileInfo, error)
	// MakeDir creates a directory (including parents if necessary).
	MakeDir(path string) error
	// Delete removes a file or directory (recursively if dir).
	Delete(path string) error
	// Rename moves src to dst.
	Rename(src, dst string) error
	// Chmod sets permissions on the given path.
	// Returns ErrPermissionsNotSupported if the server does not support it.
	Chmod(path string, mode uint32) error
	// SupportsChmod reports whether this protocol implements Chmod at all: a
	// static property of the adapter, never a probe. A server may still refuse.
	SupportsChmod() bool
	// Download opens a reader for the given file. Caller must close it.
	Download(path string) (io.ReadCloser, error)
	// Upload streams from r into the given path, overwriting if it exists.
	Upload(path string, r io.Reader) error
	// Ping verifies the underlying connection is still alive with a
	// lightweight round trip (FTP NOOP / SFTP realpath).
	Ping() error
	// Close terminates the underlying connection.
	Close() error
}

// Sentinel errors returned by adapters. Handlers check these with errors.Is.
var (
	ErrAuthFailed              = errors.New("auth failed")
	ErrConnectionFailed        = errors.New("connection failed")
	ErrPermissionsNotSupported = errors.New("permissions not supported")
	// ErrTLSFailed marks an FTPS handshake or certificate failure, kept distinct
	// from a connection failure so the API can hint at insecure-skip-verify.
	ErrTLSFailed = errors.New("tls handshake failed")
	// ErrHostKeyMismatch marks an SFTP host key that does not match the pinned
	// known_hosts entry (a possible man-in-the-middle).
	ErrHostKeyMismatch = errors.New("host key mismatch")
	// ErrConnectionTimeout marks a dial or command that exceeded its deadline,
	// as opposed to being actively refused.
	ErrConnectionTimeout = errors.New("connection timed out")
	// ErrConnectionLost marks a socket that died mid-operation. Adapters set it
	// where they know it is one, so classify need not guess from message text.
	ErrConnectionLost = errors.New("connection lost")
	ErrNotFound       = errors.New("not found")
	ErrFileExists     = errors.New("file exists")
	// ErrPermissionDenied is set structurally by the adapters, so handlers never
	// have to match server text (a path can contain "permission denied" too).
	ErrPermissionDenied = errors.New("permission denied")
	ErrQuotaExceeded    = errors.New("quota exceeded")
	ErrDirNotEmpty      = errors.New("directory not empty")
	// ErrDataConnectionFailed marks an FTP data channel that could not be
	// established or died early: usually passive mode, NAT, or a firewall.
	ErrDataConnectionFailed = errors.New("data connection failed")
	// ErrTransferIncomplete marks a transfer that ended early, so the
	// destination is in a partial state rather than untouched.
	ErrTransferIncomplete = errors.New("transfer incomplete")
	// ErrHostKeyStoreUnavailable marks a known_hosts file that could not be
	// read or written, which must never be reported as an auth failure.
	ErrHostKeyStoreUnavailable = errors.New("host key store unavailable")
	// ErrSubsystemUnavailable marks an SSH login that succeeded against a
	// server providing no SFTP subsystem.
	ErrSubsystemUnavailable = errors.New("sftp subsystem unavailable")
	ErrInvalidType          = errors.New("invalid type")
)

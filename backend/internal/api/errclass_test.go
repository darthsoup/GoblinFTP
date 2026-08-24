package api

import (
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"syscall"
	"testing"

	"github.com/pkg/sftp"
	"github.com/stretchr/testify/assert"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode gftperrors.Code
	}{
		{"nil", nil, gftperrors.ErrOperationFailed},
		{"generic", errors.New("something weird happened"), gftperrors.ErrOperationFailed},

		// Structural sentinels from the adapters.
		{"sentinel not found", fmt.Errorf("%w: x", transfer.ErrNotFound), gftperrors.ErrFileNotFound},
		{"sentinel permission", fmt.Errorf("%w: x", transfer.ErrPermissionDenied), gftperrors.ErrFilePermission},
		{"sentinel dir not empty", fmt.Errorf("%w: x", transfer.ErrDirNotEmpty), gftperrors.ErrDirNotEmpty},
		{"sentinel quota", fmt.Errorf("%w: x", transfer.ErrQuotaExceeded), gftperrors.ErrQuotaExceeded},
		{"sentinel exists", fmt.Errorf("%w: x", transfer.ErrFileExists), gftperrors.ErrFileExists},
		{"sentinel data conn", fmt.Errorf("%w: x", transfer.ErrDataConnectionFailed), gftperrors.ErrDataConnectionFailed},
		{"sentinel incomplete", fmt.Errorf("%w: x", transfer.ErrTransferIncomplete), gftperrors.ErrTransferIncomplete},
		{"sentinel timeout", fmt.Errorf("%w: x", transfer.ErrConnectionTimeout), gftperrors.ErrConnectionTimeout},
		{"sentinel chmod unsupported", transfer.ErrPermissionsNotSupported, gftperrors.ErrPermissionsNotSupported},
		{"sentinel invalid type", fmt.Errorf("%w: x", transfer.ErrInvalidType), gftperrors.ErrInvalidType},
		{"sentinel conn lost", fmt.Errorf("%w: x", transfer.ErrConnectionLost), gftperrors.ErrConnectionLost},

		// Text fallback, for servers whose only signal is the wording.
		{"text not empty", errors.New("rmdir: directory not empty"), gftperrors.ErrDirNotEmpty},
		{"text permission", errors.New("550 Permission denied"), gftperrors.ErrFilePermission},
		{"text not found", errors.New("550 No such file or directory"), gftperrors.ErrFileNotFound},
		{"text quota", errors.New("552 Disk full"), gftperrors.ErrQuotaExceeded},
		{"text exists", errors.New("550 File already exists"), gftperrors.ErrFileExists},

		{"socket broken pipe", errors.New("write: broken pipe"), gftperrors.ErrConnectionLost},
		{"deadline exceeded", os.ErrDeadlineExceeded, gftperrors.ErrConnectionTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, msg := classify(tt.err)
			assert.Equal(t, tt.wantCode, code)
			assert.NotEmpty(t, msg)
			if tt.err != nil {
				// The friendly message must never echo the raw protocol string.
				assert.NotContains(t, msg, tt.err.Error())
			}
		})
	}
}

// The old classifier substring-matched "552"/"553" and the connection-loss words
// against the whole error string, so a remote path could pick the code.
func TestClassifyIgnoresRemotePathText(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode gftperrors.Code
	}{
		{
			"path containing 553",
			fmt.Errorf("%w: /reports/2553/q1.csv", transfer.ErrNotFound),
			gftperrors.ErrFileNotFound,
		},
		{
			"path containing 552",
			fmt.Errorf("%w: /archive/5521/data.bin", transfer.ErrNotFound),
			gftperrors.ErrFileNotFound,
		},
		{
			"filename containing connection lost",
			fmt.Errorf("stat /logs/connection lost.txt: %w", transfer.ErrNotFound),
			gftperrors.ErrFileNotFound,
		},
		{
			"filename containing permission denied",
			fmt.Errorf("stat /notes/permission denied.md: %w", transfer.ErrNotFound),
			gftperrors.ErrFileNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _ := classify(tt.err)
			assert.Equal(t, tt.wantCode, code)
			assert.False(t, isConnLost(tt.err), "must not be treated as a dead connection")
		})
	}
}

func TestIsConnLost(t *testing.T) {
	assert.False(t, isConnLost(nil))
	assert.True(t, isConnLost(io.EOF))
	assert.True(t, isConnLost(syscall.EPIPE))
	assert.True(t, isConnLost(fmt.Errorf("wrapped: %w", transfer.ErrConnectionLost)))

	// A PathError wrapping a real socket error is still a lost connection: it is
	// how pkg/sftp reports one. Only the text rules are skipped for PathError.
	assert.True(t, isConnLost(&os.PathError{Op: "read", Path: "/x", Err: io.ErrUnexpectedEOF}))

	// ...so a benign failure on a path merely named like one stays benign.
	assert.False(t, isConnLost(&os.PathError{
		Op: "open", Path: "/logs/connection reset.txt", Err: transfer.ErrNotFound,
	}))
	assert.False(t, isConnLost(transfer.ErrNotFound))
}

// errors.Is against the exported fxerr values never matches a *StatusError, so
// ERR_PERMISSIONS_NOT_SUPPORTED was unreachable over SFTP until FxCode was used.
func TestSFTPStatusErrorIsNotComparableToFxerr(t *testing.T) {
	statusErr := &sftp.StatusError{Code: 8} // SSH_FX_OP_UNSUPPORTED
	assert.False(t, errors.Is(statusErr, sftp.ErrSSHFxOpUnsupported),
		"if this ever passes, the FxCode workaround in internal/sftp can be dropped")
	assert.Equal(t, sftp.ErrSSHFxOpUnsupported, statusErr.FxCode())
}

func TestTextprotoReplyCodesAreStructural(t *testing.T) {
	// classify sees these only after an adapter has wrapped them, which is what
	// keeps the bare reply number out of the substring rules.
	err := fmt.Errorf("%w: %w", transfer.ErrDataConnectionFailed,
		&textproto.Error{Code: 425, Msg: "Can't open data connection"})
	code, _ := classify(err)
	assert.Equal(t, gftperrors.ErrDataConnectionFailed, code)
}

func TestMessageForNeverEmpty(t *testing.T) {
	for _, code := range []gftperrors.Code{
		gftperrors.ErrConnectionLost, gftperrors.ErrTransferIncomplete,
		gftperrors.ErrDataConnectionFailed, gftperrors.Code("ERR_MADE_UP"),
	} {
		assert.NotEmpty(t, messageFor(code), "code=%s", code)
	}
}

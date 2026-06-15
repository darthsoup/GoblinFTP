package api

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	gftperrors "github.com/darthsoup/goblinftp/internal/errors"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode gftperrors.Code
	}{
		{"ftp remove dir failed", errors.New(`550 "Remove directory operation failed."`), gftperrors.ErrDirNotEmpty},
		{"explicit not empty", errors.New("rmdir: directory not empty"), gftperrors.ErrDirNotEmpty},
		{"ftp permission", errors.New("550 Permission denied"), gftperrors.ErrFilePermission},
		{"sftp permission", errors.New(`sftp: "Permission denied" (SSH_FX_PERMISSION_DENIED)`), gftperrors.ErrFilePermission},
		{"ftp not found", errors.New("550 No such file or directory"), gftperrors.ErrFileNotFound},
		{"sftp no such file", errors.New(`sftp: "No such file" (SSH_FX_NO_SUCH_FILE)`), gftperrors.ErrFileNotFound},
		{"quota disk full", errors.New("552 Disk full"), gftperrors.ErrQuotaExceeded},
		{"conn lost", errors.New("write: broken pipe"), gftperrors.ErrConnectionLost},
		{"generic", errors.New("something weird happened"), gftperrors.ErrOperationFailed},
		{"nil", nil, gftperrors.ErrOperationFailed},
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

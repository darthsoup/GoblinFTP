package transfer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestCleanDeletePath(t *testing.T) {
	for _, p := range []string{"", ".", "..", "file", "/", "//", "/a/..", "/a/../file", "/.. ", "/. ", "/...", "/sub/.. ", `\\server/share`, `C:/file`, "/file\t", "/file\x7f", "/file\u0085", "/file\x00", "/file\r\n"} {
		t.Run(p, func(t *testing.T) {
			_, err := transfer.CleanDeletePath(p)
			assert.ErrorIs(t, err, transfer.ErrInvalidType)
		})
	}
	for _, p := range []string{"/file", "/a//file", "/ file ", `\/file`, `\folder`} {
		t.Run(p, func(t *testing.T) {
			got, err := transfer.CleanDeletePath(p)
			require.NoError(t, err)
			assert.Equal(t, p, got)
		})
	}
	require.NoError(t, transfer.ValidateRemotePath(`/dir\..`, false, false))
}

func TestChildNamesRejectControlCharacters(t *testing.T) {
	for _, windows := range []bool{false, true} {
		for _, name := range []string{"file\t", "file\x7f", "file\u0085"} {
			require.Error(t, transfer.ValidateChildName(name, windows))
		}
	}
}

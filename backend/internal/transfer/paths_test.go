package transfer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestCleanDeletePath(t *testing.T) {
	for _, p := range []string{"", ".", "..", "../..", "a/../..", "file", "/", "//", "/a/..", " / ", `\folder`, `/folder\..`, "/file\x00", "/file\r\n"} {
		t.Run(p, func(t *testing.T) {
			_, err := transfer.CleanDeletePath(p)
			assert.ErrorIs(t, err, transfer.ErrInvalidType)
		})
	}
	for _, p := range []string{"/file", "/a/../file", "/a//file", "/ file "} {
		t.Run(p, func(t *testing.T) {
			got, err := transfer.CleanDeletePath(p)
			require.NoError(t, err)
			want := "/file"
			switch p {
			case "/a//file":
				want = "/a/file"
			case "/ file ":
				want = p
			}
			assert.Equal(t, want, got)
		})
	}
}

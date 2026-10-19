package ftp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestDeleteRejectsInvalidPathsBeforeConnecting(t *testing.T) {
	for _, p := range []string{"/", "/child/..", "..", "../..", "relative", `/dir\..`} {
		assert.ErrorIs(t, (&Client{}).Delete(p), transfer.ErrInvalidType)
	}
}

// backend/internal/transfer/token_test.go
package transfer_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

func TestTokenRoundTrip(t *testing.T) {
	secret := []byte("supersecret")
	sessionID := "sess-abc"
	path := "/some/path/file.txt"
	expiry := time.Now().Add(5 * time.Minute)

	tok := transfer.IssueToken(secret, sessionID, path, expiry)
	assert.NotEmpty(t, tok)

	gotSession, gotPath, err := transfer.ValidateToken(secret, tok)
	require.NoError(t, err)
	assert.Equal(t, sessionID, gotSession)
	assert.Equal(t, path, gotPath)
}

func TestTokenExpired(t *testing.T) {
	secret := []byte("supersecret")
	tok := transfer.IssueToken(secret, "s", "/f", time.Now().Add(-1*time.Second))

	_, _, err := transfer.ValidateToken(secret, tok)
	assert.ErrorIs(t, err, transfer.ErrTokenExpired)
}

func TestTokenTampered(t *testing.T) {
	secret := []byte("supersecret")
	tok := transfer.IssueToken(secret, "s", "/f", time.Now().Add(time.Minute))

	_, _, err := transfer.ValidateToken([]byte("wrong"), tok)
	assert.ErrorIs(t, err, transfer.ErrTokenInvalid)
}

func TestTokenMalformed(t *testing.T) {
	secret := []byte("supersecret")
	_, _, err := transfer.ValidateToken(secret, "notavalidtoken")
	assert.ErrorIs(t, err, transfer.ErrTokenInvalid)
}

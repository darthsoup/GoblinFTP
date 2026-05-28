package auth_test

import (
	"testing"

	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCSRFToken(t *testing.T) {
	token, err := auth.GenerateCSRFToken()
	require.NoError(t, err)
	assert.Len(t, token, 64, "32 bytes hex-encoded = 64 chars")
}

func TestCSRFTokensAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := auth.GenerateCSRFToken()
		require.NoError(t, err)
		assert.False(t, seen[token], "duplicate CSRF token at iteration %d", i)
		seen[token] = true
	}
}

func TestValidateCSRFTokenMatch(t *testing.T) {
	token, err := auth.GenerateCSRFToken()
	require.NoError(t, err)
	assert.True(t, auth.ValidateCSRFToken(token, token))
}

func TestValidateCSRFTokenMismatch(t *testing.T) {
	t1, _ := auth.GenerateCSRFToken()
	t2, _ := auth.GenerateCSRFToken()
	assert.False(t, auth.ValidateCSRFToken(t1, t2))
}

func TestValidateCSRFTokenEmptyInputs(t *testing.T) {
	token, _ := auth.GenerateCSRFToken()
	assert.False(t, auth.ValidateCSRFToken("", token))
	assert.False(t, auth.ValidateCSRFToken(token, ""))
	assert.False(t, auth.ValidateCSRFToken("", ""))
}

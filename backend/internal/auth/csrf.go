package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
)

const (
	CSRFTokenLength = 32
	// CSRFSessionKey is the key used to store the CSRF token in a session's Data map.
	CSRFSessionKey = "_csrf_token"
	// CSRFHeaderName is the HTTP request header name for CSRF token submission.
	CSRFHeaderName = "X-CSRF-Token"
)

// GenerateCSRFToken returns a new random 32-byte hex-encoded CSRF token.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, CSRFTokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ValidateCSRFToken compares two CSRF tokens in constant time.
// Returns true only if both are non-empty and byte-equal.
func ValidateCSRFToken(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

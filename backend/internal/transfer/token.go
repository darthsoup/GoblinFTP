// backend/internal/transfer/token.go
package transfer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrTokenExpired = errors.New("token expired")
	ErrTokenInvalid = errors.New("token invalid")
)

// IssueToken creates a signed download token.
// Format (before outer base64): subject:base64url(path):expiryUnix:hexHMAC
//
// The token is signed, not encrypted: base64 is trivially reversible, so
// subject must never be a credential. Callers pass a session's DownloadKey,
// which identifies the session without being usable as one.
func IssueToken(secret []byte, subject, path string, expiry time.Time) string {
	encodedPath := base64.RawURLEncoding.EncodeToString([]byte(path))
	expiryStr := strconv.FormatInt(expiry.Unix(), 10)
	message := subject + ":" + encodedPath + ":" + expiryStr
	mac := computeHMAC(secret, message)
	raw := message + ":" + mac
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ValidateToken verifies the HMAC, checks expiry, and returns (subject, path).
func ValidateToken(secret []byte, token string) (subject, path string, err error) {
	rawBytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", "", ErrTokenInvalid
	}
	// Exactly 4 parts: subject, base64url(path), expiryUnix, hexHMAC
	parts := strings.SplitN(string(rawBytes), ":", 4)
	if len(parts) != 4 {
		return "", "", ErrTokenInvalid
	}
	encodedPath, expiryStr, gotMAC := parts[1], parts[2], parts[3]
	subject = parts[0]

	message := subject + ":" + encodedPath + ":" + expiryStr
	expectedMAC := computeHMAC(secret, message)
	if !hmac.Equal([]byte(gotMAC), []byte(expectedMAC)) {
		return "", "", ErrTokenInvalid
	}

	expiryUnix, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return "", "", ErrTokenInvalid
	}
	if time.Now().Unix() > expiryUnix {
		return "", "", ErrTokenExpired
	}

	pathBytes, err := base64.RawURLEncoding.DecodeString(encodedPath)
	if err != nil {
		return "", "", ErrTokenInvalid
	}
	return subject, string(pathBytes), nil
}

func computeHMAC(secret []byte, message string) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

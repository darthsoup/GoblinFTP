package sso

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/hkdf"
)

var (
	ErrTokenInvalid = errors.New("invalid SSO token")
	ErrTokenExpired = errors.New("SSO token has expired")
)

// Payload is the decrypted SSO token content
type Payload struct {
	Type             string `json:"type"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	InitialDirectory string `json:"initialDirectory"`
	Language         string `json:"language,omitempty"`
	Tenant           string `json:"tenant,omitempty"`
	Exp              int64  `json:"exp"`
}

func deriveKey(secret []byte) ([]byte, error) {
	hkdfReader := hkdf.New(sha256.New, secret, nil, []byte("gftp-sso"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// Encrypt encrypts a Payload into a base64url-encoded token
func Encrypt(p *Payload, secret []byte) (string, error) {
	plaintext, err := json.Marshal(p) //nolint:gosec // G117: the payload is AES-256-GCM encrypted right below
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal payload", ErrTokenInvalid)
	}

	key, err := deriveKey(secret)
	if err != nil {
		return "", fmt.Errorf("%w: key derivation failed", ErrTokenInvalid)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("%w: failed to create cipher", ErrTokenInvalid)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("%w: failed to create GCM", ErrTokenInvalid)
	}

	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("%w: failed to generate IV", ErrTokenInvalid)
	}

	sealed := gcm.Seal(nil, iv, plaintext, nil)

	// sealed = ciphertext||tag (tag is last 16 bytes)
	tagSize := gcm.Overhead() // 16 bytes
	ciphertext := sealed[:len(sealed)-tagSize]
	tag := sealed[len(sealed)-tagSize:]

	// Wire format: iv||tag||ciphertext
	wire := make([]byte, 0, len(iv)+len(tag)+len(ciphertext))
	wire = append(wire, iv...)
	wire = append(wire, tag...)
	wire = append(wire, ciphertext...)

	token := base64.RawURLEncoding.EncodeToString(wire)
	return token, nil
}

// Decrypt decrypts a base64url-encoded token into a Payload
func Decrypt(raw string, secret []byte) (*Payload, error) {
	wire, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 encoding", ErrTokenInvalid)
	}

	// Check minimum length: 12 (IV) + 16 (tag) + 1 (ciphertext) = 29 bytes
	if len(wire) < 29 {
		return nil, fmt.Errorf("%w: token too short", ErrTokenInvalid)
	}

	// Extract components: iv||tag||ciphertext
	iv := wire[:12]
	tag := wire[12:28]
	ciphertext := wire[28:]

	key, err := deriveKey(secret)
	if err != nil {
		return nil, fmt.Errorf("%w: key derivation failed", ErrTokenInvalid)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create cipher", ErrTokenInvalid)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create GCM", ErrTokenInvalid)
	}

	// For gcm.Open, we need ciphertext||tag (tag at end)
	combined := make([]byte, 0, len(ciphertext)+len(tag))
	combined = append(combined, ciphertext...)
	combined = append(combined, tag...)

	plaintext, err := gcm.Open(nil, iv, combined, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: decryption failed", ErrTokenInvalid)
	}

	var p Payload
	if err := json.Unmarshal(plaintext, &p); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal payload", ErrTokenInvalid)
	}

	if time.Now().Unix() > p.Exp {
		return nil, fmt.Errorf("%w", ErrTokenExpired)
	}

	return &p, nil
}

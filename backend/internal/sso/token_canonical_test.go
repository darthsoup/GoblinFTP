package sso

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDecryptRequiresCanonicalEncoding(t *testing.T) {
	secret := []byte("test-secret-key-minimum-32-bytes!")
	payload := &Payload{Type: "ftp", Host: "example.com", Exp: time.Now().Add(time.Hour).Unix()}
	for range 3 {
		token, err := Encrypt(payload, secret)
		require.NoError(t, err)
		_, err = Decrypt(token, secret)
		require.NoError(t, err)

		variants := map[string]string{
			"leading_cr":    "\r" + token,
			"embedded_lf":   token[:8] + "\n" + token[8:],
			"trailing_crlf": token + "\r\n",
			"leading_space": " " + token,
			"trailing_tab":  token + "\t",
			"padding":       token + "=",
		}
		if len(token)%4 != 0 {
			const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
			last := strings.IndexByte(alphabet, token[len(token)-1])
			alias := token[:len(token)-1] + string(alphabet[last|1])
			canonicalWire, err := base64.RawURLEncoding.DecodeString(token)
			require.NoError(t, err)
			aliasWire, err := base64.RawURLEncoding.DecodeString(alias)
			require.NoError(t, err)
			require.Equal(t, canonicalWire, aliasWire)
			variants["nonzero_trailing_bits"] = alias
		}
		for name, raw := range variants {
			t.Run(name+"_"+string(rune('0'+len(token)%4)), func(t *testing.T) {
				_, err := Decrypt(raw, secret)
				require.ErrorIs(t, err, ErrTokenInvalid)
			})
		}
		payload.Username += "x"
	}
}

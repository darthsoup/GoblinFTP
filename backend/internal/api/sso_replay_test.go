package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/sso"
)

func TestSSOLoginRejectsAlternateTokenEncodings(t *testing.T) {
	cfg := ssoEnabledConfig()
	app, _, _ := newTestApp(t, cfg)
	payload := &sso.Payload{Type: "ftp", Host: "ftp.example.com", Port: 21, Exp: time.Now().Add(time.Hour).Unix()}
	token, err := sso.Encrypt(payload, cfg.SSOSecret)
	require.NoError(t, err)
	for len(token)%4 == 0 {
		payload.Username += "x"
		token, err = sso.Encrypt(payload, cfg.SSOSecret)
		require.NoError(t, err)
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, token[len(token)-1])
	variants := []string{
		"\r" + token,
		token[:8] + "\n" + token[8:],
		token + "\r\n",
		token[:len(token)-1] + string(alphabet[last|1]),
	}
	login := func(raw string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/?sso="+url.QueryEscape(raw), nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		return rec
	}
	for _, raw := range variants {
		rec := login(raw)
		assert.Equal(t, "/login?sso_error=invalid", rec.Header().Get("Location"))
		assert.Empty(t, rec.Result().Cookies())
	}
	rec := login(token)
	require.Equal(t, "/login?sso_pending=1", rec.Header().Get("Location"))
	require.NotEmpty(t, rec.Result().Cookies())
	for _, raw := range variants {
		rec := login(raw)
		assert.Equal(t, "/login?sso_error=invalid", rec.Header().Get("Location"))
		assert.Empty(t, rec.Result().Cookies())
	}
	assert.Equal(t, "/login?sso_error=used", login(token).Header().Get("Location"))
}

func TestSSOLoginConcurrentReplayMintsOneSession(t *testing.T) {
	cfg := ssoEnabledConfig()
	app, _, _ := newTestApp(t, cfg)
	token := validSSO(t, cfg.SSOSecret)
	var successes atomic.Int32
	var reused atomic.Int32
	var minted atomic.Int32
	var done sync.WaitGroup
	start := make(chan struct{})
	for range 16 {
		done.Go(func() {
			<-start
			req := httptest.NewRequest(http.MethodGet, "/?sso="+token, nil)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)
			switch rec.Header().Get("Location") {
			case "/login?sso_pending=1":
				successes.Add(1)
			case "/login?sso_error=used":
				reused.Add(1)
			}
			if len(rec.Result().Cookies()) > 0 {
				minted.Add(1)
			}
		})
	}
	close(start)
	done.Wait()
	assert.EqualValues(t, 1, successes.Load())
	assert.EqualValues(t, 15, reused.Load())
	assert.EqualValues(t, 1, minted.Load())
}

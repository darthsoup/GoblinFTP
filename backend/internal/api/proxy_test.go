package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

func allowlistConfig(t *testing.T, allowed, trustedProxies []string) *config.Config {
	t.Helper()
	cfg := defaultTestConfig()
	cfg.Settings.Access.AllowedClientAddresses = allowed
	cfg.Settings.Access.TrustedProxies = trustedProxies
	return cfg
}

// Echo's default RealIP() trusts any X-Forwarded-For, which would put the client
// allowlist one header away from useless when no trusted proxy is configured.
func TestClientAllowlistIgnoresSpoofedXFF(t *testing.T) {
	tests := []struct {
		name           string
		allowed        []string
		trustedProxies []string
		remoteAddr     string
		xff            string
		wantAllowed    bool
	}{
		{
			name:        "spoofed XFF cannot satisfy the allowlist",
			allowed:     []string{"203.0.113.9"},
			remoteAddr:  "198.51.100.20:5555",
			xff:         "203.0.113.9",
			wantAllowed: false,
		},
		{
			name:        "direct peer on the allowlist is admitted",
			allowed:     []string{"198.51.100.20"},
			remoteAddr:  "198.51.100.20:5555",
			wantAllowed: true,
		},
		{
			name:        "CIDR entries match",
			allowed:     []string{"198.51.100.0/24"},
			remoteAddr:  "198.51.100.77:5555",
			wantAllowed: true,
		},
		{
			name:        "CIDR entries do not over-match",
			allowed:     []string{"198.51.100.0/24"},
			remoteAddr:  "198.51.101.77:5555",
			wantAllowed: false,
		},
		{
			// Behind a declared proxy the forwarded client is the real one.
			// Otherwise every client collapses to the proxy's address.
			name:           "trusted proxy forwards the real client",
			allowed:        []string{"203.0.113.9"},
			trustedProxies: []string{"198.51.100.0/24"},
			remoteAddr:     "198.51.100.20:5555",
			xff:            "203.0.113.9",
			wantAllowed:    true,
		},
		{
			name:           "an untrusted hop in the chain is not believed",
			allowed:        []string{"203.0.113.9"},
			trustedProxies: []string{"198.51.100.0/24"},
			remoteAddr:     "203.0.113.250:5555",
			xff:            "203.0.113.9",
			wantAllowed:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &testutil.MockClient{WorkingDirFn: func() (string, error) { return "/", nil }}
			cfg := allowlistConfig(t, tc.allowed, tc.trustedProxies)
			app, _, _ := newTestApp(t, cfg, api.WithDial(staticDial(mock)))
			app.IPExtractor = api.IPExtractor(cfg)

			req := newConnectRequest(t, connectPayload{
				Protocol: "ftp", Host: "ftp.example", Port: 21,
				Username: "user", Password: "pass",
			})
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			if tc.wantAllowed {
				assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
			} else {
				assert.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
			}
		})
	}
}

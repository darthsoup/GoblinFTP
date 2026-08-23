package sentry

import (
	"net/http"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
)

const testDSN = "https://public@o0.ingest.sentry.io/0"

// TestRequestSecretsAreFiltered pins the one property keeping credentials out of
// Sentry: it scrubs by KEY NAME, so ?sso= is filtered but ?ticket= would not be.
func TestRequestSecretsAreFiltered(t *testing.T) {
	if err := Init(Options{DSN: testDSN, Environment: "test", Release: "v0"}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	r, err := http.NewRequest(http.MethodGet,
		"http://gftp.example/?sso=SSO_TOKEN_WITH_PASSWORD&token=DOWNLOAD_TOKEN&path=/pub", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Cookie", "gftp_session=SESSION_ID_VALUE")
	r.Header.Set("X-CSRF-Token", "CSRF_VALUE")
	r.Header.Set("Authorization", "Bearer BEARER_VALUE")

	req := sentry.NewRequest(r)

	// Everything the request carried, flattened into what would leave the process.
	var b strings.Builder
	b.WriteString(req.URL)
	b.WriteString(req.QueryString)
	b.WriteString(req.Cookies)
	b.WriteString(req.Data)
	for k, v := range req.Headers {
		b.WriteString(k)
		b.WriteString(v)
	}
	serialized := b.String()

	secrets := []string{
		"SSO_TOKEN_WITH_PASSWORD",
		"DOWNLOAD_TOKEN",
		"SESSION_ID_VALUE",
		"CSRF_VALUE",
		"BEARER_VALUE",
	}
	for _, secret := range secrets {
		if strings.Contains(serialized, secret) {
			t.Errorf("secret %q reached the Sentry request payload: %s", secret, serialized)
		}
	}

	// Guards against the test passing because nothing is collected at all: a
	// non-sensitive param must still survive, or this asserts nothing.
	if !strings.Contains(serialized, "path=") {
		t.Errorf("non-sensitive query param was dropped, test would pass vacuously: %s", serialized)
	}
}

// TestSampleRateIsNotCoerced guards the documented GFTP_SENTRY_SAMPLE_RATE default:
// an earlier version rewrote 0 to 1.0, so "no tracing" traced every transaction.
func TestSampleRateIsNotCoerced(t *testing.T) {
	if err := Init(Options{DSN: testDSN, Environment: "test", Release: "v0"}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := sentry.CurrentHub().Client().Options().TracesSampleRate; got != 0 {
		t.Errorf("TracesSampleRate = %v, want 0 (the documented default)", got)
	}
}

// TestInitWithoutDSNIsNoOp documents that every exported function stays safe
// when Sentry was never configured, which is the default deployment.
func TestInitWithoutDSNIsNoOp(t *testing.T) {
	if err := Init(Options{}); err != nil {
		t.Errorf("Init with empty DSN returned %v, want nil", err)
	}
}

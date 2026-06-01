// backend/internal/sentry/sentry_test.go
package sentry_test

import (
	"testing"

	gftpsentry "github.com/darthsoup/goblinftp/internal/sentry"
	"github.com/stretchr/testify/assert"
)

func TestInit_NoDSN(t *testing.T) {
	// Empty DSN is a no-op — must not return an error.
	assert.NoError(t, gftpsentry.Init("", "test", "v0.0.1", 1.0))
}

func TestFlush_Noop(t *testing.T) {
	// Flush with no Sentry client must not panic.
	gftpsentry.Flush()
}

func TestMiddleware_Passthrough(t *testing.T) {
	// When Sentry is not initialised, Middleware returns a pass-through — must not panic.
	mw := gftpsentry.Middleware()
	assert.NotNil(t, mw)
}

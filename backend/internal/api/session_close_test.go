package api_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/api"
	"github.com/darthsoup/goblinftp/internal/auth"
	"github.com/darthsoup/goblinftp/internal/config"
	"github.com/darthsoup/goblinftp/internal/transfer/testutil"
)

// shortTTLConfig expires sessions almost immediately so the eviction sweep can
// be driven directly instead of waiting on the janitor's interval.
func shortTTLConfig() *config.Config {
	cfg := defaultTestConfig()
	cfg.SessionTTLSeconds = 1
	return cfg
}

// The TTL sweep used to drop expired sessions with a bare map delete, leaking the
// control connection until the remote server timed it out.
func TestExpiredSessionClosesClient(t *testing.T) {
	mock := &testutil.MockClient{WorkingDirFn: func() (string, error) { return "/", nil }}
	app, store, _ := newTestApp(t, shortTTLConfig(), api.WithDial(staticDial(mock)))
	connectAndGetSession(t, app)
	require.False(t, mock.IsClosed(), "a live session must keep its connection")

	time.Sleep(1100 * time.Millisecond)
	store.EvictExpired()

	assert.True(t, mock.IsClosed(), "an expired session must close its transfer client")
}

// The shutdown hook must close every live connection rather than let process
// death sever it.
func TestEvictAllClosesEveryClient(t *testing.T) {
	mock := &testutil.MockClient{WorkingDirFn: func() (string, error) { return "/", nil }}
	app, store, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	connectAndGetSession(t, app)

	store.EvictAll()

	assert.True(t, mock.IsClosed(), "shutdown must close live transfer clients")
	assert.Equal(t, 0, store.Count())
}

// The sweep must never close a connection out from under a running transfer; the
// session is left in place for a later sweep instead.
func TestInFlightTransferIsNotEvicted(t *testing.T) {
	mock := &testutil.MockClient{WorkingDirFn: func() (string, error) { return "/", nil }}
	app, store, _ := newTestApp(t, shortTTLConfig(), api.WithDial(staticDial(mock)))
	connectAndGetSession(t, app)

	var held *auth.Session
	store.Range(func(s *auth.Session) { held = s })
	require.NotNil(t, held)

	held.LockTransfer() // stands in for a transfer in progress
	time.Sleep(1100 * time.Millisecond)
	store.EvictExpired()
	assert.False(t, mock.IsClosed(), "a transfer in flight must not have its client closed")

	held.UnlockTransfer()
	store.EvictExpired()
	assert.True(t, mock.IsClosed(), "once the transfer finishes, the next sweep collects it")
}

// Close used to panic on a second call, unlike its sibling sso.UsedSet.Stop.
func TestStoreCloseIsIdempotent(t *testing.T) {
	_, store, _ := newTestApp(t, defaultTestConfig())
	store.Close()
	store.Close()
}

// A download holds the transfer lock for the whole stream and e.Shutdown does not
// kill it, so a blocking LockTransfer here would hang shutdown until SIGKILL.
func TestEvictAllIsBoundedByAStuckTransfer(t *testing.T) {
	mock := &testutil.MockClient{WorkingDirFn: func() (string, error) { return "/", nil }}
	app, store, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	connectAndGetSession(t, app)

	var stuck *auth.Session
	store.Range(func(s *auth.Session) { stuck = s })
	require.NotNil(t, stuck)

	stuck.LockTransfer() // a transfer that never completes
	defer stuck.UnlockTransfer()

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		store.EvictAll()
		done <- time.Since(start)
	}()

	select {
	case took := <-done:
		assert.Less(t, took, 30*time.Second, "EvictAll must be bounded, not wait on the transfer")
		assert.True(t, mock.IsClosed(), "shutdown closes the client even if the transfer never finished")
	case <-time.After(30 * time.Second):
		t.Fatal("EvictAll blocked on a stuck transfer: shutdown would hang until SIGKILL")
	}
}

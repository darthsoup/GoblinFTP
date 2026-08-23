// backend/internal/api/session_close_test.go
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

// TestExpiredSessionClosesClient is the regression test for the main long-run
// leak: the TTL sweep dropped expired sessions with a bare map delete, so an
// abandoned browser tab left its FTP/SFTP control connection open until the
// remote server timed it out. The explicit disconnect path closed it; the
// expiry path did not.
func TestExpiredSessionClosesClient(t *testing.T) {
	mock := &testutil.MockClient{WorkingDirFn: func() (string, error) { return "/", nil }}
	app, store, _ := newTestApp(t, shortTTLConfig(), api.WithDial(staticDial(mock)))
	connectAndGetSession(t, app)
	require.False(t, mock.IsClosed(), "a live session must keep its connection")

	time.Sleep(1100 * time.Millisecond)
	store.EvictExpired()

	assert.True(t, mock.IsClosed(), "an expired session must close its transfer client")
}

// TestEvictAllClosesEveryClient covers the shutdown hook: at exit every live
// connection is closed rather than severed by process death.
func TestEvictAllClosesEveryClient(t *testing.T) {
	mock := &testutil.MockClient{WorkingDirFn: func() (string, error) { return "/", nil }}
	app, store, _ := newTestApp(t, defaultTestConfig(), api.WithDial(staticDial(mock)))
	connectAndGetSession(t, app)

	store.EvictAll()

	assert.True(t, mock.IsClosed(), "shutdown must close live transfer clients")
	assert.Equal(t, 0, store.Count())
}

// TestInFlightTransferIsNotEvicted is the safety property: the sweep must never
// close a connection out from under a running transfer. The session is left in
// place for a later sweep instead.
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

// TestStoreCloseIsIdempotent - Close used to panic on a second call, unlike its
// sibling sso.UsedSet.Stop.
func TestStoreCloseIsIdempotent(t *testing.T) {
	_, store, _ := newTestApp(t, defaultTestConfig())
	store.Close()
	store.Close()
}

// TestEvictAllIsBoundedByAStuckTransfer is the regression test for a shutdown
// hang. /api/files/download is public, outside requireSession, and holds the
// transfer lock for the whole stream; e.Shutdown returns after its grace period
// but does not kill that goroutine. A blocking LockTransfer here meant the
// process never finished shutting down and died only on SIGKILL.
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

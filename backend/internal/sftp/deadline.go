package sftp

import (
	"context"
	"net"
	"sync"
	"time"
)

const idleTimeout = time.Minute

// SSH reads continuously, so deadlines are armed only during SFTP operations.
type activityConn struct {
	net.Conn
	mu        sync.Mutex
	limit     time.Time
	refreshed time.Time
}

func (c *activityConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.progress()
	}
	return n, err
}

func (c *activityConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.progress()
	}
	return n, err
}

func (c *activityConn) progress() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.limit.IsZero() || now.Sub(c.refreshed) < time.Second {
		return
	}
	deadline := now.Add(idleTimeout)
	if deadline.After(c.limit) {
		deadline = c.limit
	}
	_ = c.SetDeadline(deadline)
	c.refreshed = now
}

func (c *activityConn) begin(ctx context.Context) func() {
	c.mu.Lock()
	now := time.Now()
	// Bound each RPC even if a peer sends SSH keepalives without an SFTP reply.
	c.limit = now.Add(5 * time.Minute)
	c.refreshed = now
	_ = c.SetDeadline(now.Add(idleTimeout))
	c.mu.Unlock()
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		c.mu.Lock()
		c.limit = time.Now().Add(time.Second)
		_ = c.SetDeadline(c.limit)
		c.mu.Unlock()
		close(done)
	})
	return func() {
		if !stop() {
			<-done
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		c.limit = time.Time{}
		_ = c.SetDeadline(time.Time{})
	}
}

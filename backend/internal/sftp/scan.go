package sftp

import (
	"context"
	"fmt"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

func (c *Client) LstatContext(ctx context.Context, p string) (transfer.FileInfo, error) {
	finish, err := c.scanGuard(ctx)
	if err != nil {
		return transfer.FileInfo{}, err
	}
	info, err := c.sftp.LstatContext(ctx, p)
	if err = finish(err); err != nil {
		return transfer.FileInfo{}, err
	}
	return infoFromFS(info), nil
}

func (c *Client) ReadDirContext(ctx context.Context, p string, limit int) ([]transfer.FileInfo, bool, error) {
	finish, err := c.scanGuard(ctx)
	if err != nil {
		return nil, false, err
	}
	entries, truncated, err := c.sftp.ReadDirLimitContext(ctx, p, limit)
	if err = finish(err); err != nil {
		return nil, false, err
	}
	out := make([]transfer.FileInfo, 0, len(entries))
	for _, entry := range entries {
		out = append(out, infoFromFS(entry))
	}
	return out, truncated, nil
}

func (c *Client) scanGuard(ctx context.Context) (func(error) error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	canceled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		// Closing the transport also interrupts blocked writes and handle cleanup.
		_ = c.transport.Close()
		close(canceled)
	})
	return func(err error) error {
		if !stop() {
			<-canceled
		}
		if ctx.Err() != nil {
			_ = c.transport.Close()
			return fmt.Errorf("%w: %w", transfer.ErrConnectionLost, ctx.Err())
		}
		return wrapErr(err)
	}, nil
}

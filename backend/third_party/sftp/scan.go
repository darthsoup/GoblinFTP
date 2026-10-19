package sftp

import (
	"context"
	"io"
	"os"
	"path"
)

// LstatContext reads metadata without following links and obeys ctx.
func (c *Client) LstatContext(ctx context.Context, p string) (os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id := c.nextID()
	typ, data, err := c.sendPacket(ctx, nil, &sshFxpLstatPacket{ID: id, Path: p})
	if err != nil {
		return nil, err
	}
	switch typ {
	case sshFxpAttrs:
		sid, data, err := unmarshalUint32Safe(data)
		if err != nil {
			return nil, err
		}
		if sid != id {
			return nil, &unexpectedIDErr{id, sid}
		}
		attr, _, err := unmarshalAttrs(data)
		if err != nil {
			return nil, err
		}
		return fileInfoFromStat(attr, path.Base(p)), nil
	case sshFxpStatus:
		return nil, normaliseError(unmarshalStatus(id, data))
	default:
		return nil, unimplementedPacketErr(typ)
	}
}

// ReadDirLimitContext retains at most limit entries and reports unread contents.
func (c *Client) ReadDirLimitContext(ctx context.Context, p string, limit int) (entries []os.FileInfo, truncated bool, err error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if limit <= 0 {
		return nil, true, nil
	}
	handle, err := c.opendir(ctx, p)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		if closeErr := c.closeContext(ctx, handle); err == nil {
			err = closeErr
		}
	}()
	entries = make([]os.FileInfo, 0, min(limit, 128))
	for {
		if err := ctx.Err(); err != nil {
			return entries, false, err
		}
		id := c.nextID()
		typ, data, err := c.sendPacket(ctx, nil, &sshFxpReaddirPacket{ID: id, Handle: handle})
		if err != nil {
			return entries, false, err
		}
		switch typ {
		case sshFxpName:
			sid, data, err := unmarshalUint32Safe(data)
			if err != nil {
				return entries, false, err
			}
			if sid != id {
				return entries, false, &unexpectedIDErr{id, sid}
			}
			count, data, err := unmarshalUint32Safe(data)
			if err != nil {
				return entries, false, err
			}
			for range count {
				filename, rest, err := unmarshalStringSafe(data)
				if err != nil {
					return entries, false, err
				}
				_, rest, err = unmarshalStringSafe(rest)
				if err != nil {
					return entries, false, err
				}
				attr, rest, err := unmarshalAttrs(rest)
				if err != nil {
					return entries, false, err
				}
				data = rest
				if filename == "." || filename == ".." {
					continue
				}
				if len(entries) == limit {
					return entries, true, nil
				}
				entries = append(entries, fileInfoFromStat(attr, filename))
			}
		case sshFxpStatus:
			err := normaliseError(unmarshalStatus(id, data))
			if err == io.EOF {
				return entries, false, nil
			}
			return entries, false, err
		default:
			return entries, false, unimplementedPacketErr(typ)
		}
	}
}

func (c *Client) closeContext(ctx context.Context, handle string) error {
	id := c.nextID()
	typ, data, err := c.sendPacket(ctx, nil, &sshFxpClosePacket{ID: id, Handle: handle})
	if err != nil {
		return err
	}
	if typ != sshFxpStatus {
		return unimplementedPacketErr(typ)
	}
	return normaliseError(unmarshalStatus(id, data))
}

package api

import (
	"encoding/binary"
	"fmt"
	"io"
)

const maxArchiveEntries = 10000
const maxArchiveMetadata = 4 << 20

// Validate directory records before archive/zip allocates a File for each entry.
func preflightZip(r io.ReaderAt, size int64) error {
	bad := func() error { return fmt.Errorf("%w: invalid or excessive ZIP metadata", errArchiveCorrupt) }
	if size < 22 {
		return bad()
	}
	tailSize := int64(65557)
	if tailSize > size {
		tailSize = size
	}
	tail := make([]byte, tailSize)
	if _, err := r.ReadAt(tail, size-tailSize); err != nil {
		return bad()
	}
	off := -1
	for i := len(tail) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(tail[i:]) == 0x06054b50 && i+22+int(binary.LittleEndian.Uint16(tail[i+20:])) == len(tail) {
			off = i
			break
		}
	}
	if off < 0 {
		return bad()
	}
	e := tail[off:]
	end := uint64(size - tailSize + int64(off)) //nolint:gosec // G115: offsets are bounded by the positive file size and the metadata cap.
	u16 := func(i int) uint16 { return binary.LittleEndian.Uint16(e[i:]) }
	if u16(4) != 0 || u16(6) != 0 || u16(8) != u16(10) {
		return bad()
	}
	count := uint64(u16(10))
	bytes := uint64(binary.LittleEndian.Uint32(e[12:]))
	start := uint64(binary.LittleEndian.Uint32(e[16:]))
	if count == 65535 || bytes == 0xffffffff || start == 0xffffffff {
		if end < 20 {
			return bad()
		}
		locator := make([]byte, 20)
		if _, err := r.ReadAt(locator, int64(end-20)); err != nil { //nolint:gosec // G115: offsets are bounded by the positive file size and the metadata cap.
			return bad()
		}
		if binary.LittleEndian.Uint32(locator) != 0x07064b50 || binary.LittleEndian.Uint32(locator[4:]) != 0 || binary.LittleEndian.Uint32(locator[16:]) != 1 {
			return bad()
		}
		zoff := binary.LittleEndian.Uint64(locator[8:])
		if zoff > end-20 || end-20-zoff < 56 {
			return bad()
		}
		z := make([]byte, 56)
		if _, err := r.ReadAt(z, int64(zoff)); err != nil { //nolint:gosec // G115: offsets are bounded by the positive file size and the metadata cap.
			return bad()
		}
		recordSize := binary.LittleEndian.Uint64(z[4:])
		if binary.LittleEndian.Uint32(z) != 0x06064b50 || recordSize < 44 || recordSize > maxArchiveMetadata || zoff+12+recordSize != end-20 {
			return bad()
		}
		if binary.LittleEndian.Uint32(z[16:]) != 0 || binary.LittleEndian.Uint32(z[20:]) != 0 {
			return bad()
		}
		count = binary.LittleEndian.Uint64(z[32:])
		if count != binary.LittleEndian.Uint64(z[24:]) {
			return bad()
		}
		bytes = binary.LittleEndian.Uint64(z[40:])
		start = binary.LittleEndian.Uint64(z[48:])
		end = zoff
	}
	if count > maxArchiveEntries || bytes > maxArchiveMetadata || start > end || bytes != end-start {
		return bad()
	}
	sr := io.NewSectionReader(r, int64(start), int64(bytes)) //nolint:gosec // G115: offsets are bounded by the positive file size and the metadata cap.
	header := make([]byte, 46)
	for range count {
		if _, err := io.ReadFull(sr, header); err != nil || binary.LittleEndian.Uint32(header) != 0x02014b50 {
			return bad()
		}
		if binary.LittleEndian.Uint16(header[34:]) != 0 {
			return bad()
		}
		extra := int64(binary.LittleEndian.Uint16(header[28:])) + int64(binary.LittleEndian.Uint16(header[30:])) + int64(binary.LittleEndian.Uint16(header[32:]))
		if _, err := io.CopyN(io.Discard, sr, extra); err != nil {
			return bad()
		}
	}
	pos, _ := sr.Seek(0, io.SeekCurrent)
	if uint64(pos) != bytes { //nolint:gosec // G115: offsets are bounded by the positive file size and the metadata cap.
		return bad()
	}
	return nil
}

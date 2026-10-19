package sftp

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	pkgSFTP "github.com/pkg/sftp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/darthsoup/goblinftp/internal/transfer"
)

func scanPipe(t *testing.T, respond func([]byte) []byte, readRequests bool) (*Client, <-chan byte) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	requests := make(chan byte, 16)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			var size uint32
			if binary.Read(serverConn, binary.BigEndian, &size) != nil {
				return
			}
			packet := make([]byte, size)
			if _, err := io.ReadFull(serverConn, packet); err != nil {
				return
			}
			requests <- packet[0]
			var response []byte
			if packet[0] == 1 {
				response = []byte{2, 0, 0, 0, 3}
			} else {
				response = respond(packet)
			}
			if response != nil {
				if binary.Write(serverConn, binary.BigEndian, uint32(len(response))) != nil {
					return
				}
				if _, err := serverConn.Write(response); err != nil {
					return
				}
			}
			if !readRequests {
				return
			}
		}
	}()
	client, err := pkgSFTP.NewClientPipe(clientConn, clientConn)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = client.Close()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("test SFTP server did not stop")
		}
	})
	return &Client{sftp: client, transport: clientConn}, requests
}

func scanStatus(packet []byte, code uint32) []byte {
	response := append([]byte{101}, packet[1:5]...)
	response = binary.BigEndian.AppendUint32(response, code)
	return append(response, 0, 0, 0, 0, 0, 0, 0, 0)
}

func scanHandle(packet []byte) []byte {
	return append(append([]byte{102}, packet[1:5]...), 0, 0, 0, 1, 'h')
}

func awaitScanRequest(t *testing.T, requests <-chan byte, expected byte) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case packet := <-requests:
			if packet == expected {
				return
			}
		case <-timer.C:
			t.Fatalf("SFTP operation %d did not start", expected)
		}
	}
}

func TestScanCancellationStopsSilentSFTP(t *testing.T) {
	for stage, packetType := range map[string]byte{"metadata": 7, "listing": 12, "cleanup": 4} {
		t.Run(stage, func(t *testing.T) {
			client, requests := scanPipe(t, func(packet []byte) []byte {
				switch packet[0] {
				case 11:
					return scanHandle(packet)
				case 12:
					if stage == "cleanup" {
						return scanStatus(packet, 1)
					}
				}
				return nil
			}, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if stage == "metadata" {
					_, err := client.LstatContext(ctx, "/data")
					done <- err
				} else {
					_, _, err := client.ReadDirContext(ctx, "/data", 10)
					done <- err
				}
			}()
			awaitScanRequest(t, requests, packetType)
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
				require.ErrorIs(t, err, transfer.ErrConnectionLost)
			case <-time.After(time.Second):
				t.Fatal("canceled scan retained the connection")
			}
		})
	}
}

func TestScanDeadlineStopsBlockedWrite(t *testing.T) {
	client, _ := scanPipe(t, nil, false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := client.LstatContext(ctx, "/data")
		done <- err
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorIs(t, err, transfer.ErrConnectionLost)
	case <-time.After(time.Second):
		t.Fatal("scan deadline did not stop a blocked socket write")
	}
}

func scanNames(packet []byte, count int) []byte {
	response := append([]byte{104}, packet[1:5]...)
	response = binary.BigEndian.AppendUint32(response, uint32(count))
	for i := range count {
		name := string(rune('a' + i))
		response = binary.BigEndian.AppendUint32(response, uint32(len(name)))
		response = append(response, name...)
		response = binary.BigEndian.AppendUint32(response, 0)
		response = binary.BigEndian.AppendUint32(response, 5)
		response = binary.BigEndian.AppendUint64(response, 1)
		response = binary.BigEndian.AppendUint32(response, 0o100644)
	}
	return response
}

func TestScanReadDirLimitStopsBeforeAnotherBatch(t *testing.T) {
	for _, count := range []int{2, 100} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			var reads atomic.Int32
			client, _ := scanPipe(t, func(packet []byte) []byte {
				switch packet[0] {
				case 11:
					return scanHandle(packet)
				case 12:
					if reads.Add(1) == 1 {
						return scanNames(packet, count)
					}
					return scanStatus(packet, 1)
				case 4:
					return scanStatus(packet, 0)
				case 7:
					response := append([]byte{105}, packet[1:5]...)
					response = binary.BigEndian.AppendUint32(response, 4)
					return binary.BigEndian.AppendUint32(response, 0o40755)
				}
				return nil
			}, true)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			entries, truncated, err := client.ReadDirContext(ctx, "/data", 2)
			require.NoError(t, err)
			require.Len(t, entries, 2)
			assert.Equal(t, count > 2, truncated)
			if count > 2 {
				assert.Equal(t, int32(1), reads.Load())
			} else {
				assert.Equal(t, int32(2), reads.Load())
			}
			info, err := client.LstatContext(ctx, "/data")
			require.NoError(t, err)
			assert.True(t, info.IsDir)
		})
	}
}

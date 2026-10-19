package ftp

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/textproto"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrScanConnectionClosed = errors.New("FTP connection closed during directory metadata operation")
	ErrListLimit            = errors.New("directory metadata inspection limit reached")
	ErrListEntry            = errors.New("directory listing contains an unsupported entry")
)

const scanReplyLimit = 64 * 1024

type directoryOperation struct {
	client *ServerConn
	ctx    context.Context
	mu     sync.Mutex
	data   net.Conn
	closed atomic.Bool
	stop   func() bool
	done   chan struct{}
}

func (c *ServerConn) beginDirectoryOperation(ctx context.Context) (*directoryOperation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	op := &directoryOperation{client: c, ctx: ctx, done: make(chan struct{})}
	op.stop = context.AfterFunc(ctx, func() {
		op.closeConnection()
		close(op.done)
	})
	return op, nil
}

func (op *directoryOperation) closeConnection() {
	op.closed.Store(true)
	_ = op.client.netConn.Close()
	op.mu.Lock()
	if op.data != nil {
		_ = op.data.Close()
	}
	op.mu.Unlock()
}

func (op *directoryOperation) setData(conn net.Conn) {
	op.mu.Lock()
	defer op.mu.Unlock()
	op.data = conn
	if op.closed.Load() {
		_ = conn.Close()
	}
}

func (op *directoryOperation) finish(err error) error {
	if !op.stop() {
		<-op.done
	}
	if op.closed.Load() {
		return errors.Join(ErrScanConnectionClosed, op.ctx.Err(), err)
	}
	return err
}

func (op *directoryOperation) command(expected int, format string, args ...interface{}) (int, string, error) {
	if err := checkForCommandInjection(format, args...); err != nil {
		return 0, "", err
	}
	if _, err := op.client.conn.Cmd(format, args...); err != nil {
		op.closeConnection()
		return 0, "", err
	}
	return op.response(expected)
}

func (op *directoryOperation) response(expected int) (int, string, error) {
	var reply strings.Builder
	terminator := ""
	for {
		line, err := op.client.conn.R.ReadSlice('\n')
		if err != nil || reply.Len()+len(line) > scanReplyLimit {
			op.closeConnection()
			if err == nil {
				err = errors.New("FTP metadata response exceeds its size limit")
			}
			return 0, "", err
		}
		if reply.Len() == 0 {
			if len(line) < 4 {
				op.closeConnection()
				return 0, "", errors.New("invalid FTP metadata response")
			}
			terminator = string(line[:3]) + " "
		}
		reply.Write(line)
		if strings.HasPrefix(string(line), terminator) {
			break
		}
	}
	return textproto.NewReader(bufio.NewReader(strings.NewReader(reply.String()))).ReadResponse(expected)
}

func (op *directoryOperation) openData(command, dir string) (net.Conn, error) {
	c := op.client
	if err := checkForCommandInjection("%s %s", command, dir); err != nil {
		return nil, err
	}
	if c.usePRET {
		if _, _, err := op.command(-1, "PRET %s %s", command, dir); err != nil {
			return nil, err
		}
	}
	host, port, err := op.dataAddress()
	if err != nil {
		return nil, err
	}
	raw, err := c.options.dialer.DialContext(op.ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	op.setData(raw)
	conn := raw
	var tlsConn *tls.Conn
	if c.options.tlsConfig != nil {
		tlsConn = tls.Client(raw, c.options.tlsConfig)
		conn = tlsConn
	}
	code, msg, err := op.command(-1, "%s %s", command, dir)
	if err != nil {
		_ = raw.Close()
		return nil, err
	}
	if code != StatusAlreadyOpen && code != StatusAboutToSend {
		_ = raw.Close()
		return nil, &textproto.Error{Code: code, Msg: msg}
	}
	if tlsConn != nil {
		if err := tlsConn.HandshakeContext(op.ctx); err != nil {
			op.closeConnection()
			return nil, err
		}
	}
	return conn, nil
}

func (op *directoryOperation) dataAddress() (string, int, error) {
	c := op.client
	if !c.options.disableEPSV && !c.skipEPSV {
		_, line, err := op.command(StatusExtendedPassiveMode, "EPSV")
		if err == nil {
			port, parseErr := parseEPSV(line)
			if parseErr == nil {
				return c.host, port, nil
			}
		}
		if op.closed.Load() {
			return "", 0, ErrScanConnectionClosed
		}
		c.skipEPSV = true
	}
	_, line, err := op.command(StatusPassiveMode, "PASV")
	if err != nil {
		return "", 0, err
	}
	return c.parsePASV(line)
}

func (op *directoryOperation) closeData(conn net.Conn) error {
	done := make(chan struct{})
	cleanup := time.AfterFunc(time.Second, func() {
		op.closeConnection()
		close(done)
	})
	defer func() {
		if !cleanup.Stop() {
			<-done
		}
	}()
	_ = conn.Close()
	code, msg, err := op.response(-1)
	if err == nil && code == StatusTransfertAborted {
		code, msg, err = op.response(-1)
	}
	if err == nil && code != StatusClosingDataConnection {
		err = &textproto.Error{Code: code, Msg: msg}
	}
	if err != nil {
		op.closeConnection()
	}
	return err
}

func (op *directoryOperation) scan(dir string, limit int, visit func(*Entry) bool) (bool, error) {
	command := "LIST -a"
	parser := parseListLine
	if op.client.mlstSupported && !op.client.options.forceListHidden {
		command = "MLSD"
		parser = parseRFC3659ListLine
	}
	conn, err := op.openData(command, dir)
	if err != nil {
		return false, err
	}
	scanner := bufio.NewScanner(conn)
	truncated := false
	var parseErr error
	count := 0
	ignored := make(map[string]bool, 2)
	for scanner.Scan() {
		entry, err := parser(scanner.Text(), time.Now(), op.client.options.location)
		dotEntry := err == nil && (entry.Name == "." || entry.Name == "..")
		if dotEntry && !ignored[entry.Name] {
			ignored[entry.Name] = true
			continue
		}
		if count == limit {
			truncated = true
			break
		}
		count++
		if err != nil || dotEntry {
			parseErr = ErrListEntry
			continue
		}
		if !visit(entry) {
			break
		}
	}
	readErr := scanner.Err()
	closeErr := op.closeData(conn)
	return truncated, errors.Join(parseErr, readErr, closeErr)
}

func (c *ServerConn) ReadDirContext(ctx context.Context, dir string, limit int) (entries []*Entry, truncated bool, err error) {
	if limit < 1 {
		return nil, true, ErrListLimit
	}
	op, err := c.beginDirectoryOperation(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { err = op.finish(err) }()
	entries = make([]*Entry, 0, min(limit, 128))
	truncated, err = op.scan(dir, limit, func(entry *Entry) bool {
		entries = append(entries, entry)
		return true
	})
	return entries, truncated, err
}

func (c *ServerConn) LstatContext(ctx context.Context, p string, limit int) (entry *Entry, err error) {
	op, err := c.beginDirectoryOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = op.finish(err) }()
	if p == "/" {
		return &Entry{Name: "/", Type: EntryTypeFolder}, nil
	}
	if c.mlstSupported {
		var code int
		var message string
		code, message, err = op.command(-1, "MLST %s", p)
		if err != nil {
			return nil, err
		}
		if code == StatusRequestedFileActionOK {
			entry = &Entry{}
			lines := strings.Split(message, "\n")
			if len(lines) < 3 {
				return nil, ErrListEntry
			}
			for _, line := range lines[1 : len(lines)-1] {
				line = strings.TrimLeft(line, " ")
				if line == "" {
					continue
				}
				entry, err = parseNextRFC3659ListLine(line, c.options.location, entry)
				if err != nil {
					return nil, err
				}
			}
			return entry, nil
		}
		if code != StatusNotImplemented && code != StatusCommandNotImplemented && code != StatusNotImplementedParameter && code != StatusBadCommand {
			return nil, &textproto.Error{Code: code, Msg: message}
		}
	}
	var truncated bool
	truncated, err = op.scan(path.Dir(p), limit, func(candidate *Entry) bool {
		if candidate.Name == path.Base(p) {
			entry = candidate
			return false
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if entry != nil {
		return entry, nil
	}
	if truncated {
		return nil, ErrListLimit
	}
	return nil, &textproto.Error{Code: StatusFileUnavailable, Msg: "No such file"}
}

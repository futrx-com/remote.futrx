//go:build linux

package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// A minimal RFC 6455 server endpoint. Container programs in the built-in
// catalog build without third-party modules, so the framing lives here; it
// covers what a terminal needs and nothing else (no extensions, no
// subprotocols).

const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA

	websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	// Keystrokes and resize messages are tiny; a megabyte also admits a paste.
	maxMessageBytes = 1 << 20
	writeTimeout    = 10 * time.Second

	closeNormal         = 1000
	closeProtocolError  = 1002
	closeMessageTooBig  = 1009
	closeInternalError  = 1011
	maxControlFrameSize = 125
)

var errMessageTooBig = errors.New("websocket message too big")

type wsConn struct {
	conn   net.Conn
	reader *bufio.Reader

	writeMu sync.Mutex
	closed  bool
}

// upgradeWebSocket completes the opening handshake and takes over the
// connection. On failure it has already written the HTTP error.
func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if r.Method != http.MethodGet ||
		!headerHasToken(r.Header, "Connection", "upgrade") ||
		!headerHasToken(r.Header, "Upgrade", "websocket") ||
		r.Header.Get("Sec-WebSocket-Version") != "13" {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return nil, errors.New("not a websocket handshake")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if decoded, err := base64.StdEncoding.DecodeString(key); err != nil || len(decoded) != 16 {
		http.Error(w, "invalid Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("invalid websocket key")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket unavailable", http.StatusInternalServerError)
		return nil, errors.New("connection cannot be hijacked")
	}
	conn, buffered, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "websocket unavailable", http.StatusInternalServerError)
		return nil, err
	}
	accept := sha1.Sum([]byte(key + websocketGUID))
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(accept[:]) + "\r\n\r\n"
	// The handshake deadlines of the HTTP server no longer apply.
	_ = conn.SetDeadline(time.Time{})
	if _, err := conn.Write([]byte(response)); err != nil {
		conn.Close()
		return nil, err
	}
	return &wsConn{conn: conn, reader: buffered.Reader}, nil
}

func headerHasToken(header http.Header, name, token string) bool {
	for _, value := range header.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// readMessage returns the next text or binary message. Pings are answered and
// pongs swallowed here. A close frame is echoed and reported as io.EOF.
func (c *wsConn) readMessage(deadline time.Duration) (opcode byte, payload []byte, err error) {
	var message []byte
	var messageOpcode byte
	fragmented := false
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(deadline))
		final, frameOpcode, frame, err := c.readFrame(maxMessageBytes - len(message))
		if err != nil {
			if errors.Is(err, errMessageTooBig) {
				c.close(closeMessageTooBig, "message too big")
			}
			return 0, nil, err
		}
		switch frameOpcode {
		case opPing:
			if err := c.writeFrame(opPong, frame); err != nil {
				return 0, nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			c.close(closeNormal, "")
			return 0, nil, io.EOF
		case opText, opBinary:
			if fragmented {
				c.close(closeProtocolError, "unfinished message")
				return 0, nil, errors.New("websocket: new message inside a fragmented one")
			}
			messageOpcode = frameOpcode
		case opContinuation:
			if !fragmented {
				c.close(closeProtocolError, "unexpected continuation")
				return 0, nil, errors.New("websocket: continuation without a message")
			}
		default:
			c.close(closeProtocolError, "unknown opcode")
			return 0, nil, fmt.Errorf("websocket: unknown opcode %#x", frameOpcode)
		}
		message = append(message, frame...)
		if final {
			return messageOpcode, message, nil
		}
		fragmented = true
	}
}

func (c *wsConn) readFrame(remaining int) (final bool, opcode byte, payload []byte, err error) {
	var head [2]byte
	if _, err = io.ReadFull(c.reader, head[:]); err != nil {
		return
	}
	final = head[0]&0x80 != 0
	opcode = head[0] & 0x0F
	// No extension was negotiated, so the reserved bits must be clear, and a
	// client must mask everything it sends.
	if head[0]&0x70 != 0 || head[1]&0x80 == 0 {
		c.close(closeProtocolError, "malformed frame")
		return false, 0, nil, errors.New("websocket: malformed frame")
	}
	length := uint64(head[1] & 0x7F)
	control := opcode >= opClose
	if control && (!final || length > maxControlFrameSize) {
		c.close(closeProtocolError, "malformed control frame")
		return false, 0, nil, errors.New("websocket: malformed control frame")
	}
	switch length {
	case 126:
		var extended [2]byte
		if _, err = io.ReadFull(c.reader, extended[:]); err != nil {
			return
		}
		length = uint64(binary.BigEndian.Uint16(extended[:]))
	case 127:
		var extended [8]byte
		if _, err = io.ReadFull(c.reader, extended[:]); err != nil {
			return
		}
		length = binary.BigEndian.Uint64(extended[:])
	}
	if !control && length > uint64(remaining) {
		return false, 0, nil, errMessageTooBig
	}
	var mask [4]byte
	if _, err = io.ReadFull(c.reader, mask[:]); err != nil {
		return
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(c.reader, payload); err != nil {
		return
	}
	for index := range payload {
		payload[index] ^= mask[index%4]
	}
	return final, opcode, payload, nil
}

func (c *wsConn) writeMessage(opcode byte, payload []byte) error {
	return c.writeFrame(opcode, payload)
}

func (c *wsConn) ping() error {
	return c.writeFrame(opPing, nil)
}

func (c *wsConn) writeFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.closed {
		return net.ErrClosed
	}
	return c.writeFrameLocked(opcode, payload)
}

func (c *wsConn) writeFrameLocked(opcode byte, payload []byte) error {
	header := make([]byte, 0, 10)
	header = append(header, 0x80|opcode)
	switch {
	case len(payload) < 126:
		header = append(header, byte(len(payload)))
	case len(payload) <= 0xFFFF:
		header = append(header, 126)
		header = binary.BigEndian.AppendUint16(header, uint16(len(payload)))
	default:
		header = append(header, 127)
		header = binary.BigEndian.AppendUint64(header, uint64(len(payload)))
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	_, err := c.conn.Write(payload)
	return err
}

// close sends a close frame once and drops the connection. It is safe to call
// from any goroutine and more than once.
func (c *wsConn) close(code uint16, reason string) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	payload := binary.BigEndian.AppendUint16(nil, code)
	payload = append(payload, reason...)
	_ = c.writeFrameLocked(opClose, payload)
	_ = c.conn.Close()
}

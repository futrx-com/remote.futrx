//go:build linux

package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// testClient is the browser side of the protocol: it masks what it sends, as
// a real client must.
type testClient struct {
	t      *testing.T
	conn   net.Conn
	reader *bufio.Reader
}

func startServer(t *testing.T, config sessionConfig) (*httptest.Server, *sessionManager) {
	t.Helper()
	if config.shell == nil {
		config.shell = []string{"/bin/sh"}
	}
	if config.env == nil {
		config.env = []string{"PATH=/usr/bin:/bin", "TERM=dumb", "PS1=$ "}
	}
	if config.workspace == "" {
		config.workspace = t.TempDir()
	}
	if config.detachedTTL == 0 {
		config.detachedTTL = time.Minute
	}
	if config.maxSessions == 0 {
		config.maxSessions = 8
	}
	manager := newSessionManager(config)
	server := httptest.NewServer(newHandler(manager))
	t.Cleanup(func() {
		manager.mu.Lock()
		sessions := make([]*session, 0, len(manager.sessions))
		for _, current := range manager.sessions {
			sessions = append(sessions, current)
		}
		manager.mu.Unlock()
		for _, current := range sessions {
			current.end()
		}
		server.Close()
	})
	return server, manager
}

func dial(t *testing.T, server *httptest.Server, query string, headers map[string]string) (*testClient, *http.Response) {
	t.Helper()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", target.Host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	key := make([]byte, 16)
	_, _ = rand.Read(key)
	request := "GET /ws?" + query + " HTTP/1.1\r\nHost: " + target.Host + "\r\n" +
		"Connection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + base64.StdEncoding.EncodeToString(key) + "\r\n"
	for name, value := range headers {
		request += name + ": " + value + "\r\n"
	}
	if _, err := conn.Write([]byte(request + "\r\n")); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &testClient{t: t, conn: conn, reader: reader}, response
}

func connect(t *testing.T, server *httptest.Server, query string) *testClient {
	t.Helper()
	client, response := dial(t, server, query, nil)
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d, want 101", response.StatusCode)
	}
	return client
}

func (c *testClient) send(opcode byte, payload []byte) {
	c.t.Helper()
	c.sendFrame(0x80|opcode, payload, true)
}

func (c *testClient) sendFrame(head byte, payload []byte, masked bool) {
	c.t.Helper()
	frame := []byte{head}
	maskBit := byte(0)
	if masked {
		maskBit = 0x80
	}
	switch {
	case len(payload) < 126:
		frame = append(frame, maskBit|byte(len(payload)))
	case len(payload) <= 0xFFFF:
		frame = append(frame, maskBit|126)
		frame = binary.BigEndian.AppendUint16(frame, uint16(len(payload)))
	default:
		frame = append(frame, maskBit|127)
		frame = binary.BigEndian.AppendUint64(frame, uint64(len(payload)))
	}
	if masked {
		mask := []byte{1, 2, 3, 4}
		frame = append(frame, mask...)
		for index, value := range payload {
			frame = append(frame, value^mask[index%4])
		}
	} else {
		frame = append(frame, payload...)
	}
	if _, err := c.conn.Write(frame); err != nil {
		c.t.Fatal(err)
	}
}

func (c *testClient) input(text string) {
	c.t.Helper()
	c.send(opText, []byte(fmt.Sprintf(`{"type":"input","data":%q}`, text)))
}

// frame reads one server frame. Servers never mask.
func (c *testClient) frame() (opcode byte, payload []byte, err error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var head [2]byte
	if _, err = io.ReadFull(c.reader, head[:]); err != nil {
		return
	}
	length := uint64(head[1] & 0x7F)
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
	payload = make([]byte, length)
	_, err = io.ReadFull(c.reader, payload)
	return head[0] & 0x0F, payload, err
}

// until collects output until it contains want.
func (c *testClient) until(want string) string {
	c.t.Helper()
	var output strings.Builder
	for !strings.Contains(output.String(), want) {
		opcode, payload, err := c.frame()
		if err != nil {
			c.t.Fatalf("waiting for %q: %v; output so far %q", want, err, output.String())
		}
		if opcode == opClose {
			c.t.Fatalf("closed while waiting for %q; output so far %q", want, output.String())
		}
		if opcode == opBinary {
			output.Write(payload)
		}
	}
	return output.String()
}

// closeCode reads until the server closes and returns its close code.
func (c *testClient) closeCode() uint16 {
	c.t.Helper()
	for {
		opcode, payload, err := c.frame()
		if err != nil {
			c.t.Fatalf("waiting for close: %v", err)
		}
		if opcode == opClose {
			if len(payload) < 2 {
				c.t.Fatalf("close frame without a code")
			}
			return binary.BigEndian.Uint16(payload)
		}
	}
}

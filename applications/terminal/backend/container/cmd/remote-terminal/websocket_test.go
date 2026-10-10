//go:build linux

package main

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"strings"
	"testing"
)

func TestHandshakeRequiresAWebSocketUpgrade(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	response, err := http.Get(server.URL + "/ws?session=chat-1")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.StatusCode)
	}
}

func TestHandshakeAnswersTheKeyFromTheSpecification(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/ws?session=chat-1", nil)
	request.Header.Set("Connection", "keep-alive, Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	// The sample handshake of RFC 6455 section 1.3.
	request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101", response.StatusCode)
	}
	if got := response.Header.Get("Sec-WebSocket-Accept"); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("accept = %q", got)
	}
}

func TestPingIsAnsweredWithItsPayload(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	client := connect(t, server, "session=chat-1")
	client.send(opPing, []byte("are you there"))
	for {
		opcode, payload, err := client.frame()
		if err != nil {
			t.Fatal(err)
		}
		if opcode == opPong {
			if string(payload) != "are you there" {
				t.Fatalf("pong payload = %q", payload)
			}
			return
		}
	}
}

func TestFragmentedMessageIsReassembled(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	client := connect(t, server, "session=chat-1")
	message := `{"type":"input","data":"echo fr''agmented\n"}`
	client.sendFrame(opText, []byte(message[:10]), true)
	// A control frame may arrive between the fragments of a message.
	client.send(opPing, nil)
	client.sendFrame(0x80|opContinuation, []byte(message[10:]), true)
	client.until("fragmented")
}

func TestLargeOutputUsesExtendedLengths(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	client := connect(t, server, "session=chat-1")
	client.input("head -c 70000 /dev/zero | tr '\\0' x; echo; echo do''ne\n")
	output := client.until("done")
	if count := strings.Count(output, "x"); count < 70000 {
		t.Fatalf("received %d bytes of the 70000 written", count)
	}
}

func TestClientCloseIsEchoed(t *testing.T) {
	server, manager := startServer(t, sessionConfig{})
	client := connect(t, server, "session=chat-1")
	client.send(opClose, binary.BigEndian.AppendUint16(nil, closeNormal))
	if code := client.closeCode(); code != closeNormal {
		t.Fatalf("close code = %d, want %d", code, closeNormal)
	}
	// Closing the tab leaves the shell running for a reconnect.
	waitFor(t, "the viewer to detach", func() bool {
		return viewerCount(manager, "chat-1") == 0
	})
}

func TestProtocolViolationsCloseTheConnection(t *testing.T) {
	violations := map[string]func(*testClient){
		"unmasked frame": func(client *testClient) {
			client.sendFrame(0x80|opText, []byte("{}"), false)
		},
		"reserved bit": func(client *testClient) {
			client.sendFrame(0x80|0x40|opText, []byte("{}"), true)
		},
		"unknown opcode": func(client *testClient) {
			client.sendFrame(0x80|0x3, nil, true)
		},
		"continuation without a message": func(client *testClient) {
			client.sendFrame(0x80|opContinuation, []byte("x"), true)
		},
		"fragmented control frame": func(client *testClient) {
			client.sendFrame(opPing, nil, true)
		},
		"oversized control frame": func(client *testClient) {
			client.send(opPing, bytes.Repeat([]byte("x"), 126))
		},
		"new message inside a fragmented one": func(client *testClient) {
			client.sendFrame(opText, []byte("{"), true)
			client.sendFrame(0x80|opText, []byte("}"), true)
		},
	}
	for name, violate := range violations {
		t.Run(name, func(t *testing.T) {
			server, _ := startServer(t, sessionConfig{})
			client := connect(t, server, "session=chat-1")
			violate(client)
			if code := client.closeCode(); code != closeProtocolError {
				t.Fatalf("close code = %d, want %d", code, closeProtocolError)
			}
		})
	}
}

func TestOversizedMessageIsRefused(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	client := connect(t, server, "session=chat-1")
	// Only the header is sent: the server must refuse on the declared length
	// rather than buffer the body first.
	header := []byte{0x80 | opBinary, 0x80 | 127}
	header = binary.BigEndian.AppendUint64(header, maxMessageBytes+1)
	if _, err := client.conn.Write(header); err != nil {
		t.Fatal(err)
	}
	if code := client.closeCode(); code != closeMessageTooBig {
		t.Fatalf("close code = %d, want %d", code, closeMessageTooBig)
	}
}

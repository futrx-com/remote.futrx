package smtp

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeServerBehavior controls how the fake SMTP server responds to AUTH.
type fakeServerBehavior int

const (
	behaviorAuthOK fakeServerBehavior = iota
	behaviorAuthReject
	behaviorNoGreeting
)

type fakeServerResult struct {
	authPayload string
	mailFrom    string
	dataBody    string
}

func startFakeServer(t *testing.T, behavior fakeServerBehavior) (addr string, results <-chan fakeServerResult) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	out := make(chan fakeServerResult, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		if behavior == behaviorNoGreeting {
			conn.Close()
			return
		}

		var result fakeServerResult
		w := func(s string) { conn.Write([]byte(s)) }
		reader := bufio.NewReader(conn)
		readLine := func() (string, bool) {
			line, err := reader.ReadString('\n')
			if err != nil {
				return "", false
			}
			return strings.TrimRight(line, "\r\n"), true
		}

		w("220 fake.smtp ESMTP\r\n")
		for {
			line, ok := readLine()
			if !ok {
				return
			}
			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "EHLO"):
				w("250-fake.smtp\r\n250 AUTH PLAIN\r\n")
			case strings.HasPrefix(upper, "AUTH PLAIN"):
				payload := strings.TrimSpace(line[len("AUTH PLAIN"):])
				if payload == "" {
					w("334 \r\n")
					payload, ok = readLine()
					if !ok {
						return
					}
				}
				decoded, err := base64.StdEncoding.DecodeString(payload)
				if err == nil {
					result.authPayload = string(decoded)
				}
				if behavior == behaviorAuthReject {
					w("535 5.7.8 Authentication failed\r\n")
					continue
				}
				w("235 2.7.0 Authentication successful\r\n")
			case strings.HasPrefix(upper, "MAIL FROM:"):
				result.mailFrom = line
				w("250 OK\r\n")
			case strings.HasPrefix(upper, "RCPT TO:"):
				w("250 OK\r\n")
			case upper == "DATA":
				w("354 Start mail input\r\n")
				var body strings.Builder
				for {
					dataLine, ok := readLine()
					if !ok {
						return
					}
					if dataLine == "." {
						break
					}
					body.WriteString(dataLine)
					body.WriteString("\n")
				}
				result.dataBody = body.String()
				w("250 2.0.0 OK queued\r\n")
			case upper == "QUIT":
				w("221 Bye\r\n")
				out <- result
				return
			default:
				w("500 unrecognized command\r\n")
			}
		}
	}()

	return listener.Addr().String(), out
}

func testDialer(addr string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
}

func TestClientVerify(t *testing.T) {
	t.Run("successful auth carries exactly the supplied credentials", func(t *testing.T) {
		addr, results := startFakeServer(t, behaviorAuthOK)
		client := newTestClient("localhost", addr, testDialer(addr), true)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Verify(ctx, Account{Address: "user@example.com", AppPassword: "abcd1234abcd1234"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := <-results
		want := "\x00user@example.com\x00abcd1234abcd1234"
		if result.authPayload != want {
			t.Errorf("auth payload = %q, want %q", result.authPayload, want)
		}
	})

	t.Run("535 rejection is reported without the password", func(t *testing.T) {
		addr, _ := startFakeServer(t, behaviorAuthReject)
		client := newTestClient("localhost", addr, testDialer(addr), true)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		password := "abcd1234abcd1234"
		err := client.Verify(ctx, Account{Address: "user@example.com", AppPassword: password})
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "535") {
			t.Errorf("error %q does not mention the server reply", err.Error())
		}
		if strings.Contains(err.Error(), password) {
			t.Errorf("error %q leaks the app password", err.Error())
		}
	})

	t.Run("a listener that closes without a greeting fails within the deadline", func(t *testing.T) {
		addr, _ := startFakeServer(t, behaviorNoGreeting)
		client := newTestClient("localhost", addr, testDialer(addr), true)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- client.Verify(ctx, Account{Address: "user@example.com", AppPassword: "abcd1234abcd1234"})
		}()

		select {
		case err := <-done:
			if err == nil {
				t.Fatal("expected an error")
			}
		case <-time.After(4 * time.Second):
			t.Fatal("Verify did not return within the deadline")
		}
	})
}

func TestClientSend(t *testing.T) {
	addr, results := startFakeServer(t, behaviorAuthOK)
	client := newTestClient("localhost", addr, testDialer(addr), true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	account := Account{Address: "sender@example.com", AppPassword: "abcd1234abcd1234"}
	msg := Message{From: account.Address, To: "recipient@example.com", Subject: "Test", Body: "hello world"}
	if err := client.Send(ctx, account, msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := <-results
	wantMailFrom := fmt.Sprintf("MAIL FROM:<%s>", account.Address)
	if !strings.EqualFold(result.mailFrom, wantMailFrom) {
		t.Errorf("MAIL FROM = %q, want %q", result.mailFrom, wantMailFrom)
	}
	if !strings.Contains(result.dataBody, "hello world") {
		t.Errorf("data body missing expected content: %q", result.dataBody)
	}
}

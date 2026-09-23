package smtp

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
	emaildomain "github.com/futrx-com/remote.futrx.com/internal/model/email/domain"
)

// fakeServerBehavior controls how the fake SMTP server responds to AUTH and
// STARTTLS.
type fakeServerBehavior int

const (
	behaviorAuthReject fakeServerBehavior = iota
	behaviorNoGreeting
	behaviorNoSTARTTLS
)

type fakeServerResult struct {
	authPayload  string
	loginUser    string
	loginPass    string
	mailFrom     string
	dataBody     string
	sawStartTLS  bool
	sawAuthAtAll bool
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
				switch behavior {
				case behaviorNoSTARTTLS:
					w("250-fake.smtp\r\n250 AUTH PLAIN\r\n")
				default:
					w("250-fake.smtp\r\n250-STARTTLS\r\n250 AUTH PLAIN LOGIN\r\n")
				}
			case upper == "STARTTLS":
				result.sawStartTLS = true
				w("220 go ahead\r\n")
				return // caller test does not exercise the TLS handshake here
			case strings.HasPrefix(upper, "AUTH LOGIN"):
				result.sawAuthAtAll = true
				w("334 VXNlcm5hbWU6\r\n") // "Username:"
				userLine, ok := readLine()
				if !ok {
					return
				}
				userDecoded, _ := base64.StdEncoding.DecodeString(userLine)
				result.loginUser = string(userDecoded)
				w("334 UGFzc3dvcmQ6\r\n") // "Password:"
				passLine, ok := readLine()
				if !ok {
					return
				}
				passDecoded, _ := base64.StdEncoding.DecodeString(passLine)
				result.loginPass = string(passDecoded)
				w("235 2.7.0 Authentication successful\r\n")
			case strings.HasPrefix(upper, "AUTH PLAIN"):
				result.sawAuthAtAll = true
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

func testDialTCP(addr string) DialTCP {
	return func(ctx context.Context, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
}

func failingDialTLS(context.Context, string, *tls.Config) (net.Conn, error) {
	return nil, fmt.Errorf("dialTLS should not be called in this test")
}

func plainCfg(host, addr string) emailapplication.SMTPConfiguration {
	_, port, _ := net.SplitHostPort(addr)
	var p int
	fmt.Sscanf(port, "%d", &p)
	return emailapplication.SMTPConfiguration{
		Host: host, Port: uint16(p), TLSMode: emailapplication.TLSModeSTARTTLS,
		Authentication: emailapplication.AuthenticationPlain,
		Username:       "user@example.com", Password: "abcd1234abcd1234",
		FromAddress: "user@example.com",
	}
}

func TestClientVerify(t *testing.T) {
	t.Run("successful auth carries exactly the supplied credentials", func(t *testing.T) {
		addr, results := startFakeServer(t, behaviorNoSTARTTLS)
		client := newTestClient(testDialTCP(addr), failingDialTLS, 5*time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg := plainCfg("localhost", addr)
		cfg.TLSMode = emailapplication.TLSModeNone
		if err := client.Verify(ctx, cfg); err != nil {
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
		client := newTestClient(testDialTCP(addr), failingDialTLS, 5*time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg := plainCfg("localhost", addr)
		cfg.TLSMode = emailapplication.TLSModeNone
		password := cfg.Password
		err := client.Verify(ctx, cfg)
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "535") {
			t.Errorf("error %q does not mention the server reply", err.Error())
		}
		if strings.Contains(err.Error(), password) {
			t.Errorf("error %q leaks the password", err.Error())
		}
	})

	t.Run("a listener that closes without a greeting fails within the deadline", func(t *testing.T) {
		addr, _ := startFakeServer(t, behaviorNoGreeting)
		client := newTestClient(testDialTCP(addr), failingDialTLS, 3*time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		cfg := plainCfg("localhost", addr)
		cfg.TLSMode = emailapplication.TLSModeNone
		done := make(chan error, 1)
		go func() {
			done <- client.Verify(ctx, cfg)
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

	t.Run("STARTTLS mode fails when the extension is not advertised", func(t *testing.T) {
		addr, _ := startFakeServer(t, behaviorNoSTARTTLS)
		client := newTestClient(testDialTCP(addr), failingDialTLS, 5*time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg := plainCfg("localhost", addr) // TLSModeSTARTTLS by default
		err := client.Verify(ctx, cfg)
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "does not advertise STARTTLS") {
			t.Errorf("err = %v, want a STARTTLS-not-advertised error", err)
		}
	})

	t.Run("no authentication sends no AUTH command", func(t *testing.T) {
		addr, results := startFakeServer(t, behaviorNoSTARTTLS)
		client := newTestClient(testDialTCP(addr), failingDialTLS, 5*time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg := emailapplication.SMTPConfiguration{
			Host: "localhost", TLSMode: emailapplication.TLSModeNone,
			Authentication: emailapplication.AuthenticationNone, FromAddress: "relay@example.com",
		}
		_, port, _ := net.SplitHostPort(addr)
		fmt.Sscanf(port, "%d", &cfg.Port)

		if err := client.Verify(ctx, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result := <-results
		if result.sawAuthAtAll {
			t.Error("an AUTH command was sent despite AuthenticationNone")
		}
	})

	t.Run("LOGIN responds to the username/password challenges in order", func(t *testing.T) {
		addr, results := startFakeServer(t, behaviorNoSTARTTLS)
		client := newTestClient(testDialTCP(addr), failingDialTLS, 5*time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg := plainCfg("localhost", addr)
		cfg.TLSMode = emailapplication.TLSModeNone
		cfg.Authentication = emailapplication.AuthenticationLogin

		if err := client.Verify(ctx, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result := <-results
		if result.loginUser != cfg.Username || result.loginPass != cfg.Password {
			t.Errorf("LOGIN carried user=%q pass=%q, want user=%q pass=%q", result.loginUser, result.loginPass, cfg.Username, cfg.Password)
		}
	})
}

func TestClientSend(t *testing.T) {
	addr, results := startFakeServer(t, behaviorNoSTARTTLS)
	client := newTestClient(testDialTCP(addr), failingDialTLS, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := plainCfg("localhost", addr)
	cfg.TLSMode = emailapplication.TLSModeNone
	msg := emaildomain.Message{To: "recipient@example.com", Subject: "Test", Body: "hello world"}
	if err := client.Send(ctx, cfg, msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := <-results
	wantMailFrom := fmt.Sprintf("MAIL FROM:<%s>", cfg.FromAddress)
	if !strings.EqualFold(result.mailFrom, wantMailFrom) {
		t.Errorf("MAIL FROM = %q, want %q", result.mailFrom, wantMailFrom)
	}
	if !strings.Contains(result.dataBody, "hello world") {
		t.Errorf("data body missing expected content: %q", result.dataBody)
	}
}

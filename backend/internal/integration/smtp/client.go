package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	netsmtp "net/smtp"
	"strconv"
	"time"

	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
	emaildomain "github.com/futrx-com/remote.futrx.com/internal/model/email/domain"
)

// DialTCP opens a plaintext TCP connection to addr.
type DialTCP func(ctx context.Context, addr string) (net.Conn, error)

// DialTLS opens a TLS connection to addr, verifying the certificate per
// config.
type DialTLS func(ctx context.Context, addr string, config *tls.Config) (net.Conn, error)

// errSTARTTLSNotAdvertised is returned when TLSModeSTARTTLS is configured but
// the server's EHLO response does not list the STARTTLS extension. The
// connection is never upgraded in that case, and SMTP commands never proceed
// in plaintext.
var errSTARTTLSNotAdvertised = errors.New("smtp: server does not advertise STARTTLS")

// Client speaks generic SMTP: a TCP or implicit-TLS connection, an optional
// STARTTLS upgrade, and PLAIN, LOGIN, or no authentication, entirely as
// selected per call by the caller-supplied SMTP configuration. It owns no
// provider policy and imports no service package.
type Client struct {
	dialTCP   DialTCP
	dialTLS   DialTLS
	timeout   time.Duration
	localName string
}

// New builds a Client that dials over the real network, capping every
// operation at timeout.
func New(timeout time.Duration) *Client {
	dialer := &net.Dialer{}
	return &Client{
		dialTCP: func(ctx context.Context, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", addr)
		},
		dialTLS: func(ctx context.Context, addr string, config *tls.Config) (net.Conn, error) {
			d := tls.Dialer{Config: config}
			return d.DialContext(ctx, "tcp", addr)
		},
		timeout:   timeout,
		localName: "localhost",
	}
}

// newTestClient lets this package's own tests inject dialers - a plaintext
// fake server, or a TLS listener with a trusted test certificate - without
// exposing any certificate-bypass option in the production constructor.
func newTestClient(dialTCP DialTCP, dialTLS DialTLS, timeout time.Duration) *Client {
	return &Client{dialTCP: dialTCP, dialTLS: dialTLS, timeout: timeout, localName: "localhost"}
}

func (c *Client) connect(ctx context.Context, cfg emailapplication.SMTPConfiguration) (net.Conn, *netsmtp.Client, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port)))

	deadline := time.Now().Add(c.timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	dialCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var conn net.Conn
	var err error
	if cfg.TLSMode == emailapplication.TLSModeImplicit {
		conn, err = c.dialTLS(dialCtx, addr, &tls.Config{ServerName: cfg.Host})
	} else {
		conn, err = c.dialTCP(dialCtx, addr)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("smtp: dial: %w", err)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("smtp: set deadline: %w", err)
	}

	client, err := netsmtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("smtp: new client: %w", err)
	}
	if err := client.Hello(c.localName); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("smtp: hello: %w", err)
	}

	if cfg.TLSMode == emailapplication.TLSModeSTARTTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			conn.Close()
			return nil, nil, fmt.Errorf("smtp: starttls: %w", errSTARTTLSNotAdvertised)
		}
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("smtp: starttls: %w", err)
		}
	}

	return conn, client, nil
}

func (c *Client) authenticate(client *netsmtp.Client, cfg emailapplication.SMTPConfiguration) error {
	switch cfg.Authentication {
	case emailapplication.AuthenticationNone:
		return nil
	case emailapplication.AuthenticationPlain:
		return client.Auth(netsmtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host))
	case emailapplication.AuthenticationLogin:
		return client.Auth(loginAuth{username: cfg.Username, password: cfg.Password})
	default:
		return fmt.Errorf("smtp: unsupported authentication mode %q", cfg.Authentication)
	}
}

// Verify performs a real connection and login against the server without
// sending mail. A rejection surfaces as the server's own reply text; the
// password itself never appears in a returned error.
func (c *Client) Verify(ctx context.Context, cfg emailapplication.SMTPConfiguration) error {
	conn, client, err := c.connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := c.authenticate(client, cfg); err != nil {
		return fmt.Errorf("smtp: auth: %w", err)
	}
	return client.Quit()
}

// Send authenticates (when configured) and delivers msg using cfg's From
// address as the envelope and header sender.
func (c *Client) Send(ctx context.Context, cfg emailapplication.SMTPConfiguration, msg emaildomain.Message) error {
	conn, client, err := c.connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := c.authenticate(client, cfg); err != nil {
		return fmt.Errorf("smtp: auth: %w", err)
	}

	raw, err := buildRFC5322(Message{
		From:     cfg.FromAddress,
		To:       msg.To,
		Subject:  msg.Subject,
		Body:     msg.Body,
		HTMLBody: msg.HTMLBody,
	})
	if err != nil {
		return err
	}

	if err := client.Mail(cfg.FromAddress); err != nil {
		return fmt.Errorf("smtp: mail from: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("smtp: rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		w.Close()
		return fmt.Errorf("smtp: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: close data: %w", err)
	}
	return client.Quit()
}

package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	netsmtp "net/smtp"
	"time"
)

const (
	// GmailHost is the SMTP submission host for both consumer Gmail and
	// Google Workspace accounts.
	GmailHost = "smtp.gmail.com"
	// GmailPort is Gmail's STARTTLS submission port.
	GmailPort = "587"

	dialTimeout = 15 * time.Second
)

// Account is a Gmail sender identity: an address and its 16-character app
// password.
type Account struct {
	Address     string
	AppPassword string
}

// Client speaks the Gmail SMTP conversation: STARTTLS, then AUTH PLAIN.
type Client struct {
	host string
	addr string
	dial func(ctx context.Context, network, addr string) (net.Conn, error)

	// skipTLS lets the package's own tests exercise the conversation against
	// a plaintext fake server with no certificate to present.
	skipTLS bool
}

// New builds a Client that dials smtp.gmail.com:587 over the network.
func New() *Client {
	return &Client{
		host: GmailHost,
		addr: net.JoinHostPort(GmailHost, GmailPort),
		dial: (&net.Dialer{Timeout: dialTimeout}).DialContext,
	}
}

func newTestClient(host, addr string, dial func(context.Context, string, string) (net.Conn, error), skipTLS bool) *Client {
	return &Client{host: host, addr: addr, dial: dial, skipTLS: skipTLS}
}

func (c *Client) connect(ctx context.Context) (net.Conn, *netsmtp.Client, error) {
	conn, err := c.dial(ctx, "tcp", c.addr)
	if err != nil {
		return nil, nil, fmt.Errorf("smtp: dial: %w", err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(dialTimeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("smtp: set deadline: %w", err)
	}

	client, err := netsmtp.NewClient(conn, c.host)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("smtp: new client: %w", err)
	}
	if err := client.Hello("localhost"); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("smtp: hello: %w", err)
	}
	if !c.skipTLS {
		if err := client.StartTLS(&tls.Config{ServerName: c.host}); err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("smtp: starttls: %w", err)
		}
	}
	return conn, client, nil
}

// Verify performs a real login against the server without sending mail. A
// wrong password surfaces as the server's own reply text; the app password
// itself never appears in a returned error.
func (c *Client) Verify(ctx context.Context, account Account) error {
	conn, client, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := client.Auth(netsmtp.PlainAuth("", account.Address, account.AppPassword, c.host)); err != nil {
		return fmt.Errorf("smtp: auth: %w", err)
	}
	return client.Quit()
}

// Send logs in and delivers msg.
func (c *Client) Send(ctx context.Context, account Account, msg Message) error {
	conn, client, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := client.Auth(netsmtp.PlainAuth("", account.Address, account.AppPassword, c.host)); err != nil {
		return fmt.Errorf("smtp: auth: %w", err)
	}

	raw, err := buildRFC5322(msg)
	if err != nil {
		return err
	}

	if err := client.Mail(msg.From); err != nil {
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

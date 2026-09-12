package smtp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"strings"
	"time"

	emaildomain "github.com/futrx-com/remote.futrx.com/internal/model/email/domain"
)

// Message is the wire representation of an outbound email: exactly the
// headers and body this package knows how to serialize. It carries its own
// From, unlike the service-layer Message, because the envelope sender is not
// implied by anything at this layer. When HTMLBody is non-empty the message
// is rendered as multipart/related, wrapping a multipart/alternative
// (text/plain + text/html) alongside the inline branded logo the HTML part
// references via cid:; otherwise it falls back to the original plain-text
// format.
type Message struct {
	From     string
	To       string
	Subject  string
	Body     string
	HTMLBody string
}

// buildRFC5322 renders msg as a CRLF-terminated RFC 5322 message ready to
// hand to the DATA command. No Message-ID header is generated - the
// receiving SMTP server assigns one on submission.
func buildRFC5322(msg Message) ([]byte, error) {
	if msg.From == "" {
		return nil, fmt.Errorf("smtp: message From is empty")
	}
	if msg.To == "" {
		return nil, fmt.Errorf("smtp: message To is empty")
	}
	for name, value := range map[string]string{"From": msg.From, "To": msg.To, "Subject": msg.Subject} {
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("smtp: header %s contains a line break", name)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", msg.From)
	fmt.Fprintf(&b, "To: %s\r\n", msg.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", msg.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")

	if msg.HTMLBody == "" {
		// Plain text only — original behaviour.
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		b.WriteString("\r\n")
		b.WriteString(msg.Body)
		return []byte(b.String()), nil
	}

	// Outer multipart/related: the alternative text/html+text/plain part,
	// plus the inline logo image the HTML part references via cid:.
	relatedBoundary := generateBoundary()
	fmt.Fprintf(&b, "Content-Type: multipart/related; boundary=\"%s\"\r\n", relatedBoundary)
	b.WriteString("\r\n")

	altBoundary := generateBoundary()
	fmt.Fprintf(&b, "--%s\r\n", relatedBoundary)
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n", altBoundary)
	b.WriteString("\r\n")

	// text/plain part
	fmt.Fprintf(&b, "--%s\r\n", altBoundary)
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.Body)
	b.WriteString("\r\n")

	// text/html part
	fmt.Fprintf(&b, "--%s\r\n", altBoundary)
	b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.HTMLBody)
	b.WriteString("\r\n")

	fmt.Fprintf(&b, "--%s--\r\n", altBoundary)

	// Inline logo image, embedded once per message and referenced from the
	// HTML part by Content-ID rather than a data: URI or a hosted URL.
	fmt.Fprintf(&b, "--%s\r\n", relatedBoundary)
	b.WriteString("Content-Type: image/png\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	fmt.Fprintf(&b, "Content-ID: <%s>\r\n", emaildomain.LogoContentID)
	b.WriteString("Content-Disposition: inline; filename=\"logo.png\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(base64Wrapped(emaildomain.LogoPNG))

	fmt.Fprintf(&b, "--%s--\r\n", relatedBoundary)

	return []byte(b.String()), nil
}

// base64Wrapped encodes data as base64, split into 76-character lines per
// RFC 2045 §6.8.
func base64Wrapped(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	var out strings.Builder
	out.Grow(len(encoded) + len(encoded)/76*2)
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		out.WriteString(encoded[i:end])
		out.WriteString("\r\n")
	}
	return out.String()
}

// generateBoundary returns a random MIME boundary string.
func generateBoundary() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fallback: deterministic but unique-enough for a single message.
		return "----=_Part_Remote_0001"
	}
	return "----=_Part_Remote_" + hex.EncodeToString(buf[:])
}

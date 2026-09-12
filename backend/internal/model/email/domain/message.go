// Package emaildomain holds the email bounded context's transport-agnostic
// data: the message a caller composes and address parsing shared by message
// recipients and SMTP configuration. It has no dependency on SMTP, storage,
// or HTTP.
package emaildomain

// Message is one outbound email. There is no From: the sender identity comes
// from the configured SMTP account, never from the caller. When HTMLBody is
// non-empty the sender builds a multipart/alternative message with both a
// text/plain fallback (Body) and a text/html part (HTMLBody).
type Message struct {
	To       string
	Subject  string
	Body     string
	HTMLBody string
}

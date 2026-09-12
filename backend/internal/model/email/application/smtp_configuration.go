// Package emailapplication holds the email bounded context's application
// data: the complete (secret-bearing) SMTP configuration the service
// validates, verifies, and persists, and its secret-free public projection.
package emailapplication

// TLSMode selects how the SMTP integration secures the connection.
type TLSMode string

const (
	// TLSModeSTARTTLS requires the server to advertise STARTTLS and upgrades
	// to it before any credential or message data is sent.
	TLSModeSTARTTLS TLSMode = "starttls"
	// TLSModeImplicit establishes TLS before any SMTP conversation begins.
	TLSModeImplicit TLSMode = "implicit"
	// TLSModeNone sends no TLS at all. Only valid paired with
	// AuthenticationNone, so credentials are never sent in the clear.
	TLSModeNone TLSMode = "none"
)

// AuthenticationMode selects how the SMTP integration authenticates.
type AuthenticationMode string

const (
	AuthenticationPlain AuthenticationMode = "plain"
	AuthenticationLogin AuthenticationMode = "login"
	AuthenticationNone  AuthenticationMode = "none"
)

// SMTPConfiguration is the complete, secret-bearing SMTP account the server
// sends through. It is write-only above the store and sender: nothing above
// this package ever reads Password back out for display.
type SMTPConfiguration struct {
	Host           string
	Port           uint16
	TLSMode        TLSMode
	Authentication AuthenticationMode
	Username       string
	Password       string
	FromAddress    string
}

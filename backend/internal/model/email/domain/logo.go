package emaildomain

import _ "embed"

// LogoContentID is the fixed MIME Content-ID every outbound HTML message's
// inline logo is embedded under. The rendered shell's `<img src="cid:...">`
// (service/email/layout.go) and the SMTP integration's embedded image part
// (integration/smtp/message.go) must agree on this exact value; it is
// declared here, in the one package both already depend on, so they cannot
// drift apart.
const LogoContentID = "remote-logo"

// LogoPNG is the branded logo image embedded into every outbound HTML
// message as an inline MIME part, referenced from the HTML via cid: rather
// than a data: URI (which Gmail and some other webmail clients strip on
// display) or a hosted URL (which would require the server to be publicly
// reachable and would leak a request to it whenever the message is opened).
//
//go:embed logo.png
var LogoPNG []byte

package smtp

import (
	"fmt"
	"net/smtp"
)

// loginAuth implements the SMTP AUTH LOGIN mechanism, which net/smtp does not
// provide: the server challenges for "Username:" then "Password:" in a fixed
// order, each answered with the raw value (net/smtp base64-encodes every
// response itself).
type loginAuth struct {
	username string
	password string
}

var _ smtp.Auth = loginAuth{}

func (a loginAuth) Start(_ *smtp.ServerInfo) (proto string, toServer []byte, err error) {
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch string(fromServer) {
	case "Username:":
		return []byte(a.username), nil
	case "Password:":
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("smtp: unexpected LOGIN challenge %q", fromServer)
	}
}

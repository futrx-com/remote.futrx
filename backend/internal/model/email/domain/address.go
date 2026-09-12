package emaildomain

import (
	"errors"
	"net/mail"
	"strings"
)

// ErrInvalidAddress means the given address failed net/mail parsing or
// carried a display name rather than a bare envelope address.
var ErrInvalidAddress = errors.New("email: invalid address")

// NormalizeAddress trims and lowercases addr, then requires it to parse as a
// bare envelope address. A display-name form such as "Name <a@b.com>" is
// rejected by design: the stored and addressed value is an envelope address,
// not a header. Used for both message recipients and an SMTP configuration's
// sender address.
func NormalizeAddress(addr string) (string, error) {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if addr == "" {
		return "", ErrInvalidAddress
	}
	parsed, err := mail.ParseAddress(addr)
	if err != nil || parsed.Address != addr {
		return "", ErrInvalidAddress
	}
	return addr, nil
}

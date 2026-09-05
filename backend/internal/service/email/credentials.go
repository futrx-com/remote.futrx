package email

import (
	"net/mail"
	"strings"
	"unicode"
)

// normalizeAddress trims and lowercases addr, then requires it to parse as a
// bare envelope address. A display-name form such as "Name <a@b.com>" is
// rejected by design: the stored value is an envelope address, not a header.
func normalizeAddress(addr string) (string, error) {
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

// normalize validates and canonicalizes a full Credentials value: the
// address per normalizeAddress, and the app password with every whitespace
// rune stripped and its length checked against AppPasswordLength. Gmail's UI
// presents the password as four space-separated groups, so stripping
// whitespace before the length check is what lets an admin paste it as
// shown.
func normalize(creds Credentials) (Credentials, error) {
	address, err := normalizeAddress(creds.Address)
	if err != nil {
		return Credentials{}, err
	}

	var b strings.Builder
	for _, r := range creds.AppPassword {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	password := b.String()
	if len([]rune(password)) != AppPasswordLength {
		return Credentials{}, ErrInvalidAppPassword
	}

	return Credentials{Address: address, AppPassword: password}, nil
}

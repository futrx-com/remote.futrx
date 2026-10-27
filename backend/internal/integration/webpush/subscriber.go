package webpush

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"
)

// normalizeSubscriber returns the representation webpush-go expects: an HTTPS
// contact URL or a bare email address (the dependency adds the mailto: prefix).
func normalizeSubscriber(subject string) (string, error) {
	subject = strings.TrimSpace(subject)
	if strings.HasPrefix(subject, "mailto:") {
		subject = strings.TrimPrefix(subject, "mailto:")
	}
	if address, err := mail.ParseAddress(subject); err == nil && address.Address == subject {
		return subject, nil
	}

	parsed, err := url.Parse(subject)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("vapid subject %q must be an email address or https URL", subject)
	}
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

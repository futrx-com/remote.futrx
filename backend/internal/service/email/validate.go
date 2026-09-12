package email

import (
	"net"
	"regexp"
	"strings"

	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
	emaildomain "github.com/futrx-com/remote.futrx.com/internal/model/email/domain"
)

var dnsLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func validDNSName(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if !dnsLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

// validateHost trims raw and requires a bare hostname: no scheme, path,
// query, whitespace, or embedded port. Only a valid DNS name, IPv4 address,
// or IPv6 literal (bracketed or bare) is accepted.
func validateHost(raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return "", ErrInvalidConfiguration
	}
	if strings.ContainsAny(host, " \t\r\n") || strings.Contains(host, "://") || strings.ContainsAny(host, "/?#@") {
		return "", ErrInvalidConfiguration
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		inner := host[1 : len(host)-1]
		if ip := net.ParseIP(inner); ip == nil || ip.To4() != nil {
			return "", ErrInvalidConfiguration
		}
		return inner, nil
	}
	if strings.Contains(host, ":") {
		// A bare IPv6 literal is the only legitimate reason for a colon here;
		// anything else is an embedded port.
		if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
			return host, nil
		}
		return "", ErrInvalidConfiguration
	}
	if net.ParseIP(host) != nil {
		return host, nil
	}
	if !validDNSName(host) {
		return "", ErrInvalidConfiguration
	}
	return host, nil
}

func validatePort(port uint16) error {
	if port == 0 {
		return ErrInvalidConfiguration
	}
	return nil
}

func validTLSMode(mode emailapplication.TLSMode) bool {
	switch mode {
	case emailapplication.TLSModeSTARTTLS, emailapplication.TLSModeImplicit, emailapplication.TLSModeNone:
		return true
	default:
		return false
	}
}

func validAuthenticationMode(mode emailapplication.AuthenticationMode) bool {
	switch mode {
	case emailapplication.AuthenticationPlain, emailapplication.AuthenticationLogin, emailapplication.AuthenticationNone:
		return true
	default:
		return false
	}
}

// ConfigureRequest is the service's input for Configure: the public fields
// exactly as the API layer decoded and mapped them, plus a password with
// three-state semantics - nil means omitted (retain the stored secret),
// a pointer to "" means explicitly cleared (invalid whenever authentication
// is enabled).
type ConfigureRequest struct {
	Host           string
	Port           uint16
	TLSMode        emailapplication.TLSMode
	Authentication emailapplication.AuthenticationMode
	Username       string
	Password       *string
	FromAddress    string
}

// buildCandidate validates req and produces the complete configuration to
// verify and persist. current is the presently stored configuration (nil
// when none), consulted only to resolve an omitted password.
func buildCandidate(current *emailapplication.SMTPConfiguration, req ConfigureRequest) (emailapplication.SMTPConfiguration, error) {
	host, err := validateHost(req.Host)
	if err != nil {
		return emailapplication.SMTPConfiguration{}, err
	}
	if err := validatePort(req.Port); err != nil {
		return emailapplication.SMTPConfiguration{}, err
	}
	if !validTLSMode(req.TLSMode) || !validAuthenticationMode(req.Authentication) {
		return emailapplication.SMTPConfiguration{}, ErrInvalidConfiguration
	}
	if req.TLSMode == emailapplication.TLSModeNone && req.Authentication != emailapplication.AuthenticationNone {
		// Never allow credentials to be configured for a plaintext transport.
		return emailapplication.SMTPConfiguration{}, ErrInvalidConfiguration
	}
	fromAddress, err := emaildomain.NormalizeAddress(req.FromAddress)
	if err != nil {
		return emailapplication.SMTPConfiguration{}, ErrInvalidConfiguration
	}

	candidate := emailapplication.SMTPConfiguration{
		Host:        host,
		Port:        req.Port,
		TLSMode:     req.TLSMode,
		FromAddress: fromAddress,
	}

	if req.Authentication == emailapplication.AuthenticationNone {
		candidate.Authentication = emailapplication.AuthenticationNone
		return candidate, nil
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		return emailapplication.SMTPConfiguration{}, ErrInvalidConfiguration
	}

	var password string
	switch {
	case req.Password == nil:
		if current == nil || current.Password == "" {
			return emailapplication.SMTPConfiguration{}, ErrInvalidConfiguration
		}
		password = current.Password
	case *req.Password == "":
		return emailapplication.SMTPConfiguration{}, ErrInvalidConfiguration
	default:
		password = *req.Password
	}

	candidate.Authentication = req.Authentication
	candidate.Username = username
	candidate.Password = password
	return candidate, nil
}

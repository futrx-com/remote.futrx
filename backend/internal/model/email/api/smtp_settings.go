// Package emailapi holds the wire-format DTOs for the admin SMTP settings
// HTTP boundary and the mapping between those DTOs and the canonical
// application enums. This package is the one place the JSON field names and
// enum wire values are decided.
package emailapi

import (
	emailapplication "github.com/futrx-com/remote.futrx.com/internal/model/email/application"
)

// SaveSMTPSettingsRequest is the PUT /api/admin/email request body. Password
// is a pointer so an omitted field (retain the stored secret) is
// distinguishable from an explicitly empty one (invalid whenever
// authentication is enabled).
type SaveSMTPSettingsRequest struct {
	Host           string  `json:"host"`
	Port           uint16  `json:"port"`
	TLSMode        string  `json:"tlsMode"`
	Authentication string  `json:"authentication"`
	Username       string  `json:"username"`
	Password       *string `json:"password,omitempty"`
	FromAddress    string  `json:"fromAddress"`
}

// PublicSMTPConfigurationDTO is the secret-free configuration projection
// returned by GET/PUT.
type PublicSMTPConfigurationDTO struct {
	Host               string `json:"host"`
	Port               uint16 `json:"port"`
	TLSMode            string `json:"tlsMode"`
	Authentication     string `json:"authentication"`
	Username           string `json:"username"`
	FromAddress        string `json:"fromAddress"`
	PasswordConfigured bool   `json:"passwordConfigured"`
}

// SMTPSettingsResponse is the GET/PUT response body. Configuration is
// omitted entirely when the server has none configured.
type SMTPSettingsResponse struct {
	Configured    bool                        `json:"configured"`
	Configuration *PublicSMTPConfigurationDTO `json:"configuration,omitempty"`
}

var tlsModesByWireValue = map[string]emailapplication.TLSMode{
	"starttls": emailapplication.TLSModeSTARTTLS,
	"implicit": emailapplication.TLSModeImplicit,
	"none":     emailapplication.TLSModeNone,
}

// ParseTLSMode maps a wire value to the canonical enum. ok is false for any
// unrecognized value.
func ParseTLSMode(wire string) (mode emailapplication.TLSMode, ok bool) {
	mode, ok = tlsModesByWireValue[wire]
	return mode, ok
}

var authenticationModesByWireValue = map[string]emailapplication.AuthenticationMode{
	"plain": emailapplication.AuthenticationPlain,
	"login": emailapplication.AuthenticationLogin,
	"none":  emailapplication.AuthenticationNone,
}

// ParseAuthenticationMode maps a wire value to the canonical enum. ok is
// false for any unrecognized value.
func ParseAuthenticationMode(wire string) (mode emailapplication.AuthenticationMode, ok bool) {
	mode, ok = authenticationModesByWireValue[wire]
	return mode, ok
}

// PublicConfigurationDTO maps the application's secret-free projection to its
// wire representation. A nil cfg maps to a nil DTO.
func PublicConfigurationDTO(cfg *emailapplication.PublicSMTPConfiguration) *PublicSMTPConfigurationDTO {
	if cfg == nil {
		return nil
	}
	return &PublicSMTPConfigurationDTO{
		Host:               cfg.Host,
		Port:               cfg.Port,
		TLSMode:            string(cfg.TLSMode),
		Authentication:     string(cfg.Authentication),
		Username:           cfg.Username,
		FromAddress:        cfg.FromAddress,
		PasswordConfigured: cfg.PasswordConfigured,
	}
}

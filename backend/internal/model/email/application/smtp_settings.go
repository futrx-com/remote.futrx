package emailapplication

// PublicSMTPConfiguration is what the admin-facing API and its callers are
// allowed to know about a stored configuration: everything except the
// password, represented only by PasswordConfigured.
type PublicSMTPConfiguration struct {
	Host               string
	Port               uint16
	TLSMode            TLSMode
	Authentication     AuthenticationMode
	Username           string
	FromAddress        string
	PasswordConfigured bool
}

// SMTPSettings is the settings-read result. A nil Configuration is the valid
// unconfigured state.
type SMTPSettings struct {
	Configuration *PublicSMTPConfiguration
}

// Public projects cfg into its secret-free representation. A nil cfg (no
// configuration stored) projects to a nil *PublicSMTPConfiguration.
func Public(cfg *SMTPConfiguration) *PublicSMTPConfiguration {
	if cfg == nil {
		return nil
	}
	return &PublicSMTPConfiguration{
		Host:               cfg.Host,
		Port:               cfg.Port,
		TLSMode:            cfg.TLSMode,
		Authentication:     cfg.Authentication,
		Username:           cfg.Username,
		FromAddress:        cfg.FromAddress,
		PasswordConfigured: cfg.Password != "",
	}
}

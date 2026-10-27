package config

import (
	"errors"
	"net/url"
	"os"
	"time"
)

type Config struct {
	Host         string
	Port         string
	DataDir      string
	InstallDir   string
	BaseURL      string
	Agent        AgentOptions
	Auth         AuthOptions
	Applications ApplicationOptions
}

// ApplicationOptions are application-wide settings for installable application
// backends. Toolchain discovery remains in the backend integration; config
// owns the optional environment override supplied to it.
type ApplicationOptions struct {
	GoTool string
}

// AgentOptions are application-wide policies for the agent subsystem.
type AgentOptions struct {
	// CapabilityTimeout bounds one provider's complete model/capability probe
	// (AGENT_CAPABILITY_TIMEOUT, Go duration, default 30s, "0" disables).
	CapabilityTimeout time.Duration
	// HostCLIVersionTimeout bounds each host-side CLI version probe performed
	// by the infrastructure convergence command.
	HostCLIVersionTimeout time.Duration
	// CapabilityCacheTTL retains a fully live, warning-free catalog.
	CapabilityCacheTTL time.Duration
	// DegradedCapabilityCacheTTL retries fallback or warning-bearing catalogs
	// sooner than healthy catalogs.
	DegradedCapabilityCacheTTL time.Duration
	// CredentialSyncTimeout bounds the best-effort post-run copy of refreshed
	// provider credentials from a project container back to the host.
	CredentialSyncTimeout time.Duration
	// BrowserIdleTTL controls how long an agent browser stack may remain idle
	// before the project service stops it.
	BrowserIdleTTL time.Duration
}

// AuthOptions are application-wide policies for optional account security
// features. Protocol constants such as the TOTP period and code width remain
// owned by the auth package.
type AuthOptions struct {
	// PendingLoginTTL is the lifetime of the token bridging a successful first
	// factor and the second-factor challenge.
	PendingLoginTTL time.Duration
	// EnrollmentTTL is the lifetime of a pending TOTP enrollment token.
	EnrollmentTTL time.Duration
	// RecoveryCodeCount is the number of one-time recovery codes issued as a
	// set during enrollment or regeneration.
	RecoveryCodeCount int
	// SessionHistoryLimit bounds the newest-first sign-in history per account.
	SessionHistoryLimit int
	// SetupTokenTTL bounds how long a printed first-boot setup token stays
	// usable (SETUP_TOKEN_TTL, Go duration, default 30m).
	SetupTokenTTL time.Duration
}

func Load() Config {
	return Config{
		Host:       envDefault("HOST", "127.0.0.1"),
		Port:       envDefault("PORT", "7682"),
		DataDir:    envDefault("DATA_DIR", "/opt/remote.futrx/data"),
		InstallDir: envDefault("INSTALL_DIR", "/opt/remote.futrx"),
		BaseURL:    envDefault("BASE_URL", ""),
		Agent: AgentOptions{
			CapabilityTimeout:          envDuration("AGENT_CAPABILITY_TIMEOUT", 30*time.Second),
			HostCLIVersionTimeout:      15 * time.Second,
			CapabilityCacheTTL:         24 * time.Hour,
			DegradedCapabilityCacheTTL: 2 * time.Hour,
			CredentialSyncTimeout:      30 * time.Second,
			BrowserIdleTTL:             20 * time.Minute,
		},
		Auth: AuthOptions{
			PendingLoginTTL:     5 * time.Minute,
			EnrollmentTTL:       10 * time.Minute,
			RecoveryCodeCount:   10,
			SessionHistoryLimit: 20,
			SetupTokenTTL:       envDuration("SETUP_TOKEN_TTL", 30*time.Minute),
		},
		Applications: ApplicationOptions{
			GoTool: envDefault("REMOTE_APPLICATION_GO", ""),
		},
	}
}

func (c Config) Addr() string {
	return c.Host + ":" + c.Port
}

// PublicHostname returns the hostname selected during installation.
func PublicHostname(baseURL string) (string, error) {
	parsed, err := parseBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	return parsed.Hostname(), nil
}

func parseBaseURL(baseURL string) (*url.URL, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return nil, errors.New("BASE_URL must be an absolute URL")
	}
	return parsed, nil
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envDuration parses a Go duration from the environment. Unset or invalid
// values fall back to the default; an explicit "0" disables the limit.
func envDuration(key string, def time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed < 0 {
		return def
	}
	return parsed
}

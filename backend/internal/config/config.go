package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Host       string
	Port       string
	DataDir    string
	InstallDir string
	BaseURL    string
	Agent      AgentOptions
	Auth       AuthOptions
	Schedule   ScheduleLimits
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
}

// ScheduleLimits are the scheduled-task guardrails. Zero disables a limit;
// the env values below choose conservative defaults so unattended agent runs
// cannot take the host down.
type ScheduleLimits struct {
	// MinInterval is the floor between two starts of one cron task
	// (SCHEDULE_MIN_INTERVAL, Go duration, default 5m, "0" disables).
	MinInterval time.Duration
	// MaxConcurrentRuns caps simultaneous scheduled runs across all chats
	// (SCHEDULE_MAX_CONCURRENT, default 2, "0" disables).
	MaxConcurrentRuns int
	// MaxTasksPerProject caps standing tasks per project
	// (SCHEDULE_MAX_TASKS_PER_PROJECT, default 20, "0" disables).
	MaxTasksPerProject int
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
		},
		Schedule: ScheduleLimits{
			MinInterval:        envDuration("SCHEDULE_MIN_INTERVAL", 5*time.Minute),
			MaxConcurrentRuns:  envInt("SCHEDULE_MAX_CONCURRENT", 2),
			MaxTasksPerProject: envInt("SCHEDULE_MAX_TASKS_PER_PROJECT", 20),
		},
	}
}

func (c Config) Addr() string {
	return c.Host + ":" + c.Port
}

// CodeServerBaseURL derives the IDE origin from the public hostname selected
// during installation. For example, https://remote.example.com becomes
// https://code.remote.example.com/.
func CodeServerBaseURL(baseURL string) (string, error) {
	parsed, err := parseBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	host := "code." + parsed.Hostname()
	if port := parsed.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	}
	parsed.Host = host
	parsed.Path = "/"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
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

// envInt parses a non-negative integer from the environment. Unset or invalid
// values fall back to the default; an explicit "0" disables the limit.
func envInt(key string, def int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return def
	}
	return parsed
}

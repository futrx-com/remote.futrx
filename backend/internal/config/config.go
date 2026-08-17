package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/audit"
)

type Config struct {
	Host       string
	Port       string
	DataDir    string
	InstallDir string
	BaseURL    string
	Schedule   ScheduleLimits
	Audit      AuditLimits
}

// AuditLimits are the audit-log housekeeping knobs.
type AuditLimits struct {
	// RetentionMonths is how many monthly audit files to keep, including the
	// current one (AUDIT_RETENTION_MONTHS, default 12). A value below 1
	// disables the janitor and keeps every file forever.
	RetentionMonths int
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
		Schedule: ScheduleLimits{
			MinInterval:        envDuration("SCHEDULE_MIN_INTERVAL", 5*time.Minute),
			MaxConcurrentRuns:  envInt("SCHEDULE_MAX_CONCURRENT", 2),
			MaxTasksPerProject: envInt("SCHEDULE_MAX_TASKS_PER_PROJECT", 20),
		},
		Audit: AuditLimits{
			RetentionMonths: envInt("AUDIT_RETENTION_MONTHS", audit.DefaultRetentionMonths),
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

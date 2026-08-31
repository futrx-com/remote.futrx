package opencode

// Auth configures the host-side `opencode auth login` flow for the
// opencode-ai CLI. OpenCode's login is interactive per provider (provider
// selection, then method selection; OAuth providers print a URL and a device
// code). Credentials land in ~/.local/share/opencode/auth.json.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

const (
	deviceLoginReadyTimeout = 8 * time.Second
	deviceLoginTimeout      = 30 * time.Minute
	deviceLoginTTL          = 29 * time.Minute
)

var (
	ErrOpenCodeNotFound = errors.New("opencode CLI not found on PATH - install it first")

	// OAuth providers print an authorize URL; the user code is optional and
	// provider-specific, so a lenient XXXX-XXXX pattern is used.
	deviceURLRE  = regexp.MustCompile(`https?://\S+`)
	deviceCodeRE = regexp.MustCompile(`[A-Z0-9]{4}-[A-Z0-9]{4,6}`)
)

type AuthStatus struct {
	Authenticated bool             `json:"authenticated"`
	DeviceLogin   DeviceLoginState `json:"deviceLogin,omitempty"`
}

type DeviceLoginState = agentauth.DeviceState
type Auth = agentauth.DeviceService[AuthStatus]

func NewAuth() *Auth {
	return agentauth.NewDeviceService(agentauth.DeviceConfig[AuthStatus]{
		Command:         "opencode",
		Args:            []string{"auth", "login"},
		Env:             opencodeEnv,
		NotFound:        ErrOpenCodeNotFound,
		StartErrorLabel: "opencode auth login",
		ReadyTimeout:    deviceLoginReadyTimeout,
		LoginTimeout:    deviceLoginTimeout,
		LoginTTL:        deviceLoginTTL,
		URLPattern:      deviceURLRE,
		CodePattern:     deviceCodeRE,
		Authenticated:   authenticated,
		BuildStatus: func() agentauth.DeviceStatusBuilder[AuthStatus] {
			authenticated := authenticated()
			return func(state agentauth.DeviceState) AuthStatus {
				return AuthStatus{Authenticated: authenticated, DeviceLogin: state}
			}
		},
		ResolveCompletion: func(err error) agentauth.DeviceCompletion {
			switch {
			case authenticated():
				return agentauth.DeviceCompletion{Completed: true}
			case err != nil:
				return agentauth.DeviceCompletion{Error: fmt.Sprintf("opencode auth login failed: %s", truncate(err.Error(), 300))}
			default:
				return agentauth.DeviceCompletion{Error: "OpenCode login ended before authentication completed."}
			}
		},
	})
}

// authenticated reports whether an OpenCode credential store exists on the
// host: auth.json must decode to a non-empty JSON object (the CLI keeps the
// file present but empty after `opencode auth logout`).
func authenticated() bool {
	raw, err := os.ReadFile(hostOpenCodeAuth())
	if err != nil {
		return false
	}
	var store map[string]json.RawMessage
	if err := json.Unmarshal(raw, &store); err != nil {
		return false
	}
	return len(store) > 0
}

// hostOpenCodeData resolves the OpenCode data directory (XDG_DATA_HOME aware).
func hostOpenCodeData() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "opencode")
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "share", "opencode")
	}
	return "/root/.local/share/opencode"
}

func hostOpenCodeAuth() string {
	return filepath.Join(hostOpenCodeData(), "auth.json")
}

func opencodeEnv(base []string) []string {
	// OpenCode resolves its data directory from XDG_DATA_HOME; forward it so
	// the login flow writes where the rest of the integration looks.
	for _, env := range base {
		if strings.HasPrefix(env, "XDG_DATA_HOME=") {
			return base
		}
	}
	return append(base, "XDG_DATA_HOME="+filepath.Dir(hostOpenCodeData()))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

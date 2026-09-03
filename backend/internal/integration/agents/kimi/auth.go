package kimi

// Auth configures the host-side `kimi login` device-code flow for
// @moonshot-ai/kimi-code. Unlike Codex there is no API-key mode: Kimi Code
// auth is always a subscription OAuth grant under ~/.kimi-code/credentials/.

import (
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
	ErrKimiNotFound = errors.New("kimi CLI not found on PATH - install it first")

	// kimi prints e.g.
	//   https://www.kimi.com/code/authorize_device?user_code=T906-Q0QV
	deviceURLRE  = regexp.MustCompile(`https://www\.kimi\.com/code/authorize_device\S*`)
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
		Command:         "kimi",
		Args:            []string{"login"},
		Env:             kimiEnv,
		NotFound:        ErrKimiNotFound,
		StartErrorLabel: "kimi login",
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
				return agentauth.DeviceCompletion{Error: fmt.Sprintf("kimi login failed: %s", truncate(err.Error(), 300))}
			default:
				return agentauth.DeviceCompletion{Error: "Kimi login ended before authentication completed."}
			}
		},
	})
}

// authenticated reports whether a Kimi Code OAuth credential exists on the
// host (any regular file under ~/.kimi-code/credentials/).
func authenticated() bool {
	entries, err := os.ReadDir(kimiCredsDir())
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Type().IsRegular() {
			return true
		}
	}
	return false
}

func kimiHomeDir() string {
	if v := os.Getenv("KIMI_CODE_HOME"); v != "" {
		return v
	}
	if v := os.Getenv("HOME"); v != "" {
		return filepath.Join(v, ".kimi-code")
	}
	return "/root/.kimi-code"
}

func kimiCredsDir() string {
	return filepath.Join(kimiHomeDir(), "credentials")
}

func kimiEnv(base []string) []string {
	for _, env := range base {
		if strings.HasPrefix(env, "KIMI_CODE_HOME=") {
			return base
		}
	}
	return append(base, "KIMI_CODE_HOME="+kimiHomeDir())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

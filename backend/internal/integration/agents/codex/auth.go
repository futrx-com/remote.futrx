package codex

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
	deviceLoginTimeout      = 16 * time.Minute
	deviceLoginTTL          = 15 * time.Minute
)

var (
	ErrCodexNotFound = errors.New("codex CLI not found on PATH - install it first")

	deviceURLRE  = regexp.MustCompile(`https://auth\.openai\.com/codex/device`)
	deviceCodeRE = regexp.MustCompile(`[A-Z0-9]{4}-[A-Z0-9]{5}`)
)

type AuthStatus struct {
	Authenticated bool             `json:"authenticated"`
	AuthMode      string           `json:"authMode,omitempty"`
	UsesAPIKey    bool             `json:"usesApiKey,omitempty"`
	DeviceLogin   DeviceLoginState `json:"deviceLogin,omitempty"`
}

type DeviceLoginState = agentauth.DeviceState
type Auth = agentauth.DeviceService[AuthStatus]

func NewAuth() *Auth {
	return agentauth.NewDeviceService(agentauth.DeviceConfig[AuthStatus]{
		Command:         "codex",
		Args:            []string{"login", "--device-auth"},
		Env:             codexAuthEnv,
		NotFound:        ErrCodexNotFound,
		StartErrorLabel: "codex login",
		ReadyTimeout:    deviceLoginReadyTimeout,
		LoginTimeout:    deviceLoginTimeout,
		LoginTTL:        deviceLoginTTL,
		URLPattern:      deviceURLRE,
		CodePattern:     deviceCodeRE,
		Authenticated: func() bool {
			authenticated, _, _ := authenticated()
			return authenticated
		},
		BuildStatus: func() agentauth.DeviceStatusBuilder[AuthStatus] {
			authenticated, authMode, usesAPIKey := authenticated()
			return func(state agentauth.DeviceState) AuthStatus {
				return AuthStatus{
					Authenticated: authenticated,
					AuthMode:      authMode,
					UsesAPIKey:    usesAPIKey,
					DeviceLogin:   state,
				}
			}
		},
		ResolveCompletion: func(err error) agentauth.DeviceCompletion {
			authenticated, _, usesAPIKey := authenticated()
			switch {
			case authenticated:
				return agentauth.DeviceCompletion{Completed: true}
			case usesAPIKey:
				return agentauth.DeviceCompletion{Error: "Codex is still logged in with an API key. Sign in with ChatGPT to use subscription limits."}
			case err != nil:
				return agentauth.DeviceCompletion{Error: fmt.Sprintf("codex login failed: %s", truncate(err.Error(), 300))}
			default:
				return agentauth.DeviceCompletion{Error: "Codex login ended before authentication completed."}
			}
		},
	})
}

func authenticated() (bool, string, bool) {
	authPath := filepath.Join(codexHomeDir(), "auth.json")
	authMode, usesAPIKey := codexAuthMode(authPath)
	if usesAPIKey {
		return false, authMode, true
	}
	if authMode == "" {
		return false, "", false
	}
	return true, authMode, false
}

func codexAuthMode(authPath string) (string, bool) {
	data, err := os.ReadFile(authPath)
	if err != nil {
		return "", false
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return "unknown", false
	}
	mode, _ := raw["auth_mode"].(string)
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		if _, hasAPIKey := raw["OPENAI_API_KEY"]; hasAPIKey {
			return "apikey", true
		}
		return "unknown", false
	}
	return mode, mode == "apikey"
}

func codexHomeDir() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	if v := os.Getenv("HOME"); v != "" {
		return filepath.Join(v, ".codex")
	}
	return "/root/.codex"
}

func codexAuthEnv(base []string) []string {
	out := make([]string, 0, len(base)+1)
	hasCodexHome := false
	for _, env := range base {
		if strings.HasPrefix(env, "OPENAI_API_KEY=") {
			continue
		}
		if strings.HasPrefix(env, "CODEX_HOME=") {
			hasCodexHome = true
		}
		out = append(out, env)
	}
	if hasCodexHome {
		return out
	}
	return append(out, "CODEX_HOME="+codexHomeDir())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

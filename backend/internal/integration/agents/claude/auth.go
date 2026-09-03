package claude

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

const (
	claudeLoginTimeout = 10 * time.Minute
	claudeURLReadWait  = 15 * time.Second
	claudeExitWait     = 30 * time.Second
)

var (
	ErrCodeRequired   = errors.New("code is required")
	ErrNoSession      = errors.New("no login session in progress - call /api/claude/login/start first")
	ErrClaudeNotFound = errors.New("claude CLI not found on PATH - install it first")

	claudeAuthURLRE = regexp.MustCompile(`https://claude\.com/cai/oauth/authorize\?[^\s]+`)
)

type Auth = agentauth.CodeService
type AuthStatus = agentauth.CodeStatus
type LoginState = agentauth.CodeLoginState
type StartResult = agentauth.CodeStartResult

// NewAuth configures the shared authorization-code service for the Claude CLI.
func NewAuth() *Auth {
	return agentauth.NewCodeService(agentauth.CodeConfig{
		Command:        "claude",
		Args:           []string{"auth", "login", "--claudeai"},
		URLPattern:     claudeAuthURLRE,
		LoginTimeout:   claudeLoginTimeout,
		URLReadTimeout: claudeURLReadWait,
		ExitTimeout:    claudeExitWait,
		Authenticated:  authenticated,
		NotFound:       ErrClaudeNotFound,
		CodeRequired:   ErrCodeRequired,
		NoSession:      ErrNoSession,
		Errors: agentauth.CodeErrorFormatters{
			PTYStart: func(err error) error {
				return fmt.Errorf("pty start: %w", err)
			},
			URLReadTimeout: func(wait time.Duration, output string) error {
				return fmt.Errorf(
					"did not see Anthropic OAuth URL within %s; first 500 bytes of claude output: %s",
					wait,
					output,
				)
			},
			ExitBeforeURL: func(err error, output string) error {
				message := "claude exited before printing OAuth URL"
				if err != nil {
					message += " (" + err.Error() + ")"
				}
				return fmt.Errorf("%s; output: %s", message, output)
			},
			WriteCode: func(err error) error {
				return fmt.Errorf("write to claude stdin: %w", err)
			},
			ExitTimeout: func(wait time.Duration, output string) error {
				return fmt.Errorf(
					"claude did not exit within %s after code paste; last output: %s",
					wait,
					output,
				)
			},
			Exit: func(err error, output string) error {
				return fmt.Errorf("claude exited with error: %w; output: %s", err, output)
			},
			MissingCredentials: func(output string) error {
				return fmt.Errorf(
					"claude exited cleanly but no credentials file was written; output: %s",
					output,
				)
			},
		},
	})
}

func authenticated() bool {
	for _, name := range []string{".credentials.json", "credentials.json"} {
		if _, err := os.Stat(filepath.Join(claudeHomeDir(), name)); err == nil {
			return true
		}
	}
	return false
}

func claudeHomeDir() string {
	if value := os.Getenv("CLAUDE_CONFIG_DIR"); value != "" {
		return value
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".claude")
	}
	return "/root/.claude"
}

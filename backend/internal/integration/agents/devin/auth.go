package devin

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
	devinLoginTimeout = 10 * time.Minute
	devinURLReadWait  = 15 * time.Second
	devinExitWait     = 30 * time.Second
)

var (
	ErrCodeRequired  = errors.New("code is required")
	ErrNoSession     = errors.New("no login session in progress - call /api/devin/login/start first")
	ErrDevinNotFound = errors.New("devin CLI not found on PATH - install it first")

	// devinAuthURLRE matches the manual-token-flow URL printed by
	// `devin auth login --force-manual-token-flow`.
	devinAuthURLRE = regexp.MustCompile(`https://app\.devin\.ai/auth/cli/continue\?[\w=&.-]+`)
)

type Auth = agentauth.CodeService
type AuthStatus = agentauth.CodeStatus
type LoginState = agentauth.CodeLoginState
type StartResult = agentauth.CodeStartResult

// NewAuth configures the shared authorization-code service for the Devin CLI.
// The manual token flow prints a URL, waits for the user to sign in via browser
// and paste the resulting code, then persists credentials.toml.
func NewAuth() *Auth {
	return agentauth.NewCodeService(agentauth.CodeConfig{
		Command:        "devin",
		Args:           []string{"auth", "login", "--force-manual-token-flow"},
		URLPattern:     devinAuthURLRE,
		LoginTimeout:   devinLoginTimeout,
		URLReadTimeout: devinURLReadWait,
		ExitTimeout:    devinExitWait,
		Authenticated:  authenticated,
		NotFound:       ErrDevinNotFound,
		CodeRequired:   ErrCodeRequired,
		NoSession:      ErrNoSession,
		Errors: agentauth.CodeErrorFormatters{
			PTYStart: func(err error) error {
				return fmt.Errorf("pty start: %w", err)
			},
			URLReadTimeout: func(wait time.Duration, output string) error {
				return fmt.Errorf(
					"did not see Devin auth URL within %s; first 500 bytes of devin output: %s",
					wait,
					output,
				)
			},
			ExitBeforeURL: func(err error, output string) error {
				message := "devin exited before printing auth URL"
				if err != nil {
					message += " (" + err.Error() + ")"
				}
				return fmt.Errorf("%s; output: %s", message, output)
			},
			WriteCode: func(err error) error {
				return fmt.Errorf("write to devin stdin: %w", err)
			},
			ExitTimeout: func(wait time.Duration, output string) error {
				return fmt.Errorf(
					"devin did not exit within %s after code paste; last output: %s",
					wait,
					output,
				)
			},
			Exit: func(err error, output string) error {
				return fmt.Errorf("devin exited with error: %w; output: %s", err, output)
			},
			MissingCredentials: func(output string) error {
				return fmt.Errorf(
					"devin exited cleanly but no credentials file was written; output: %s",
					output,
				)
			},
		},
	})
}

// authenticated reports whether the Devin credential file exists. Confirmed by
// `devin auth status`: credentials are stored at
// ~/.local/share/devin/credentials.toml (XDG_DATA_HOME).
func authenticated() bool {
	_, err := os.Stat(filepath.Join(devinDataDir(), "credentials.toml"))
	return err == nil
}

// devinDataDir returns the directory holding Devin's credential file. It
// respects XDG_DATA_HOME and falls back to ~/.local/share/devin, matching the
// path reported by `devin auth status`.
func devinDataDir() string {
	if value := os.Getenv("XDG_DATA_HOME"); value != "" {
		return filepath.Join(value, "devin")
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "share", "devin")
	}
	return "/root/.local/share/devin"
}

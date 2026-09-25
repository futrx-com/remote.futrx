package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

// authStatusResponse is the subset of `claude auth status --json` used to
// recognize a subscription login.
type authStatusResponse struct {
	LoggedIn         bool   `json:"loggedIn"`
	AuthMethod       string `json:"authMethod"`
	Email            string `json:"email"`
	SubscriptionType string `json:"subscriptionType"`
}

// validateAccountCredential asks the CLI, in a private config directory,
// whether credential is a usable Claude subscription login.
func validateAccountCredential(ctx context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
	if len(credential) == 0 {
		return agentauth.ValidatedAccount{}, errors.New("saved Claude credential is empty")
	}
	var stored accountCredential
	if err := json.Unmarshal(credential, &stored); err != nil {
		return agentauth.ValidatedAccount{}, errors.New("saved Claude credential is not valid JSON")
	}
	if len(stored.Credentials["claudeAiOauth"]) == 0 {
		return agentauth.ValidatedAccount{}, errors.New("saved Claude account is not a Claude subscription login")
	}
	if err := checkOAuthUsable(stored.Credentials["claudeAiOauth"], time.Now()); err != nil {
		return agentauth.ValidatedAccount{}, err
	}

	root, home, err := prepareIsolatedClaudeHome("remote-claude-validate-*")
	if err != nil {
		return agentauth.ValidatedAccount{}, fmt.Errorf("prepare Claude validation: %w", err)
	}
	defer os.RemoveAll(root)
	credentialPath := filepath.Join(home, ".credentials.json")
	configPath := filepath.Join(home, ".claude.json")
	if err := writeAccountCredential(credentialPath, configPath, credential); err != nil {
		return agentauth.ValidatedAccount{}, err
	}
	status, err := readClaudeAuthStatus(ctx, home)
	if err != nil {
		return agentauth.ValidatedAccount{}, fmt.Errorf("validate Claude account: %w", err)
	}
	if !status.LoggedIn || status.AuthMethod != "claude.ai" {
		return agentauth.ValidatedAccount{}, errors.New("Claude did not recognize a Claude subscription login")
	}
	// The CLI may refresh tokens while inspecting them; keep what it wrote.
	refreshed, err := readAccountCredential(credentialPath, configPath)
	if err != nil {
		return agentauth.ValidatedAccount{}, fmt.Errorf("read refreshed Claude credential: %w", err)
	}
	email := status.Email
	if email == "" {
		email = oauthAccountEmail(stored.OAuthAccount)
	}
	return agentauth.ValidatedAccount{Email: email, PlanType: status.SubscriptionType, Credential: refreshed}, nil
}

// checkOAuthUsable rejects a login the CLI can no longer use. `claude auth
// status` reads only local files, so an account whose access token expired
// without a refresh token would otherwise pass validation.
func checkOAuthUsable(raw json.RawMessage, now time.Time) error {
	var oauth struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
	}
	if err := json.Unmarshal(raw, &oauth); err != nil {
		return errors.New("saved Claude credential is not valid JSON")
	}
	if oauth.RefreshToken != "" {
		return nil
	}
	if oauth.AccessToken == "" || (oauth.ExpiresAt > 0 && oauth.ExpiresAt <= now.UnixMilli()) {
		return errors.New("saved Claude login has expired; reconnect this account")
	}
	return nil
}

func readClaudeAuthStatus(ctx context.Context, configDir string) (authStatusResponse, error) {
	cmd := exec.CommandContext(ctx, "claude", "auth", "status", "--json")
	cmd.Env = isolatedClaudeAuthEnvFor(os.Environ(), configDir)
	cmd.Stdin = strings.NewReader("")
	cmd.Stderr = io.Discard
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	// The CLI exits non-zero when signed out but still prints its status, so
	// the output decides the result and the exit status only explains a
	// missing one.
	runErr := cmd.Run()
	status, err := parseClaudeAuthStatus(stdout.Bytes())
	if err != nil {
		if runErr != nil {
			return authStatusResponse{}, runErr
		}
		return authStatusResponse{}, err
	}
	return status, nil
}

func parseClaudeAuthStatus(output []byte) (authStatusResponse, error) {
	output = bytes.TrimSpace(output)
	if start := bytes.IndexByte(output, '{'); start > 0 {
		output = output[start:]
	}
	var status authStatusResponse
	if err := json.Unmarshal(output, &status); err != nil {
		return authStatusResponse{}, fmt.Errorf("decode claude auth status: %w", err)
	}
	return status, nil
}

func oauthAccountEmail(profile json.RawMessage) string {
	var account struct {
		EmailAddress string `json:"emailAddress"`
	}
	if len(profile) == 0 || json.Unmarshal(profile, &account) != nil {
		return ""
	}
	return account.EmailAddress
}

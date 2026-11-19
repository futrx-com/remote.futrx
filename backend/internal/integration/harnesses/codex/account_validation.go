package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

type accountReadResponse struct {
	Account *struct {
		Type     string `json:"type"`
		Email    string `json:"email"`
		PlanType string `json:"planType"`
	} `json:"account"`
	RequiresOpenAIAuth bool `json:"requiresOpenaiAuth"`
}

func validateAccountCredential(ctx context.Context, credential json.RawMessage) (agentauth.ValidatedAccount, error) {
	if len(credential) == 0 {
		return agentauth.ValidatedAccount{}, errors.New("saved Codex credential is empty")
	}
	var raw map[string]any
	if err := json.Unmarshal(credential, &raw); err != nil {
		return agentauth.ValidatedAccount{}, errors.New("saved Codex credential is not valid JSON")
	}
	mode, usesAPIKey := codexAuthModeFromRaw(raw)
	if usesAPIKey || mode != "chatgpt" {
		return agentauth.ValidatedAccount{}, errors.New("saved Codex account is not a ChatGPT subscription login")
	}
	if credentialPath, ok := snapCodexCredentialPath(); ok {
		return validateAccountCredentialAtPath(ctx, credential, credentialPath)
	}

	root, home, err := prepareIsolatedCodexHome("remote-codex-validate-*")
	if err != nil {
		return agentauth.ValidatedAccount{}, fmt.Errorf("prepare Codex validation: %w", err)
	}
	defer os.RemoveAll(root)
	credentialPath := filepath.Join(home, "auth.json")
	if err := os.WriteFile(credentialPath, credential, 0o600); err != nil {
		return agentauth.ValidatedAccount{}, err
	}
	return inspectAccountCredential(ctx, home, credentialPath)
}

func validateAccountCredentialAtPath(ctx context.Context, credential json.RawMessage, credentialPath string) (agentauth.ValidatedAccount, error) {
	previous, readErr := os.ReadFile(credentialPath)
	hadPrevious := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return agentauth.ValidatedAccount{}, fmt.Errorf("read current Codex credential: %w", readErr)
	}
	if err := agentauth.WriteCredentialFile(credentialPath, credential); err != nil {
		return agentauth.ValidatedAccount{}, fmt.Errorf("stage Codex credential for validation: %w", err)
	}
	validated, validationErr := inspectAccountCredential(ctx, filepath.Dir(credentialPath), credentialPath)
	var restoreErr error
	if hadPrevious {
		restoreErr = agentauth.WriteCredentialFile(credentialPath, previous)
	} else {
		restoreErr = os.Remove(credentialPath)
		if errors.Is(restoreErr, os.ErrNotExist) {
			restoreErr = nil
		}
	}
	if restoreErr != nil {
		return agentauth.ValidatedAccount{}, fmt.Errorf("restore current Codex credential after validation: %w", restoreErr)
	}
	return validated, validationErr
}

func inspectAccountCredential(ctx context.Context, home, credentialPath string) (agentauth.ValidatedAccount, error) {
	account, err := readCodexAccount(ctx, home)
	if err != nil {
		return agentauth.ValidatedAccount{}, fmt.Errorf("validate Codex account: %w", err)
	}
	if account.Account == nil || account.Account.Type != "chatgpt" {
		return agentauth.ValidatedAccount{}, errors.New("Codex did not recognize a ChatGPT account")
	}
	refreshed, err := os.ReadFile(credentialPath)
	if err != nil {
		return agentauth.ValidatedAccount{}, fmt.Errorf("read refreshed Codex credential: %w", err)
	}
	return agentauth.ValidatedAccount{
		Email: account.Account.Email, PlanType: account.Account.PlanType,
		Credential: append(json.RawMessage(nil), refreshed...),
	}, nil
}

func codexAuthModeFromRaw(raw map[string]any) (string, bool) {
	mode, _ := raw["auth_mode"].(string)
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		_, usesAPIKey := raw["OPENAI_API_KEY"]
		return mode, usesAPIKey
	}
	return mode, mode == "apikey"
}

func readCodexAccount(ctx context.Context, home string) (accountReadResponse, error) {
	cmd := exec.CommandContext(ctx, "codex", "app-server")
	cmd.Env = isolatedCodexAuthEnvFor(os.Environ(), home)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return accountReadResponse{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return accountReadResponse{}, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return accountReadResponse{}, err
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	return readCodexAccountRPC(stdin, stdout)
}

func readCodexAccountRPC(stdin io.Writer, stdout io.Reader) (accountReadResponse, error) {
	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(map[string]any{
		"method": "initialize", "id": 1,
		"params": map[string]any{
			"clientInfo":   map[string]string{"name": "remote-futrx", "title": "Remote", "version": "1"},
			"capabilities": map[string]bool{"experimentalApi": true},
		},
	}); err != nil {
		return accountReadResponse{}, err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var response rpcResponse
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil || response.ID == 0 {
			continue
		}
		switch response.ID {
		case 1:
			if response.Error != nil {
				return accountReadResponse{}, fmt.Errorf("initialize: %s", response.Error.Message)
			}
			if err := encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
				return accountReadResponse{}, err
			}
			if err := encoder.Encode(map[string]any{
				"method": "account/read", "id": 2,
				"params": map[string]bool{"refreshToken": true},
			}); err != nil {
				return accountReadResponse{}, err
			}
		case 2:
			if response.Error != nil {
				return accountReadResponse{}, errors.New(response.Error.Message)
			}
			var account accountReadResponse
			if err := json.Unmarshal(response.Result, &account); err != nil {
				return accountReadResponse{}, fmt.Errorf("decode account/read: %w", err)
			}
			return account, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return accountReadResponse{}, err
	}
	return accountReadResponse{}, errors.New("Codex app-server closed before returning account status")
}

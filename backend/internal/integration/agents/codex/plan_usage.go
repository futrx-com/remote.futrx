package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/codexharness"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

var _ agent.PlanUsageReader = (*Provider)(nil)

// ReadPlanUsage asks the Codex app server for each account's rate limits
// through account/rateLimits/read, the request Codex's /status is built
// from. The app server refreshes an expired sign-in as a run would, and no
// turn is started, so a read spends none of the plan.
//
// Saved accounts are read one at a time in private CODEX_HOMEs, except while
// a chat runs on one: the chat reports that account's limits itself, and a
// read could refresh the login the chat still uses. The host login is read
// only while no saved account is active, because only then do chats without a
// pinned account run on it, and account changes wait for that read. A Snap
// build ignores CODEX_HOME, so there only the host login can be read, and it
// belongs to the active account.
func (p *Provider) ReadPlanUsage(ctx context.Context) []agent.AccountPlanUsage {
	var usages []agent.AccountPlanUsage
	readHost, hostAccountID := true, ""
	if p.accounts != nil {
		snapshot := p.accounts.AccountsSnapshot()
		if _, snap := snapCodexCredentialPath(); snap {
			hostAccountID = snapshot.ActiveAccountID
		} else {
			for _, account := range snapshot.Items {
				p.accounts.WithIdleIsolatedAccount(account.ID, func() {
					usages = append(usages, p.readSavedAccountUsage(ctx, account.ID))
				})
			}
			readHost = snapshot.ActiveAccountID == ""
		}
	}
	if readHost {
		readHostLogin := func() {
			signedIn, _, usesAPIKey := authenticated()
			if !signedIn || usesAPIKey {
				return
			}
			windows, err := readCodexRateLimits(ctx, "")
			usages = append(usages, agent.AccountPlanUsage{AccountID: hostAccountID, Windows: windows, Err: err})
		}
		if p.accounts == nil {
			readHostLogin()
		} else {
			p.accounts.WithIdleHostLogin(hostAccountID, readHostLogin)
		}
	}
	return usages
}

// readSavedAccountUsage reads one saved account's rate limits in a private
// CODEX_HOME, then offers a login the app server refreshed to the account's
// vault record by the same rules as a run.
func (p *Provider) readSavedAccountUsage(ctx context.Context, accountID string) agent.AccountPlanUsage {
	usage := agent.AccountPlanUsage{AccountID: accountID}
	saved, _, err := p.accounts.CredentialForRun(accountID)
	if err != nil {
		usage.Err = err
		return usage
	}
	root, home, err := prepareIsolatedCodexHome("remote-codex-usage-*")
	if err != nil {
		usage.Err = fmt.Errorf("prepare Codex usage read: %w", err)
		return usage
	}
	defer os.RemoveAll(root)
	credentialPath := filepath.Join(home, "auth.json")
	if err := os.WriteFile(credentialPath, saved.Credential, 0o600); err != nil {
		usage.Err = err
		return usage
	}

	usage.Windows, usage.Err = readCodexRateLimits(ctx, home)
	if usage.Err == nil && len(usage.Windows) == 0 {
		usage.Err = errors.New("Codex did not report plan limits for this account")
	}

	if refreshed, err := os.ReadFile(credentialPath); err == nil {
		captureCtx, cancel := context.WithTimeout(context.Background(), p.credentialSyncTimeout)
		if err := p.accounts.CaptureRunCredential(captureCtx, saved, refreshed); err != nil {
			log.Printf("codex plan usage: keep refreshed login for account %s: %v", accountID, err)
		}
		cancel()
	}
	return usage
}

// readCodexRateLimits starts an app server on home, or on the host login when
// home is empty, and returns the Codex windows its rate-limit read reports.
func readCodexRateLimits(ctx context.Context, home string) ([]agent.Quota, error) {
	ctx, cancel := context.WithTimeout(ctx, configconstants.PlanUsageAccountReadTimeout)
	defer cancel()

	cmd := exec.Command("codex", "app-server")
	if home == "" {
		cmd.Env = codexAuthEnv(os.Environ())
	} else {
		cmd.Env = isolatedCodexAuthEnvFor(os.Environ(), home)
	}
	var result json.RawMessage
	_, err := agentruntime.Converse(ctx, cmd, configconstants.CodexPlanUsageExitGrace, func(stdin io.Writer, stdout io.Reader) error {
		var err error
		result, err = requestCodexRateLimits(stdin, stdout)
		return err
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("Codex usage read timed out: %w", ctx.Err())
		}
		return nil, err
	}
	return codexharness.RateLimitQuotas(result, time.Now().UnixMilli()), nil
}

// requestCodexRateLimits initializes the app server and returns the raw
// result of one account/rateLimits/read.
func requestCodexRateLimits(stdin io.Writer, stdout io.Reader) (json.RawMessage, error) {
	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(map[string]any{
		"method": "initialize", "id": 1,
		"params": map[string]any{
			"clientInfo":   map[string]string{"name": "remote-futrx", "title": "Remote", "version": "1"},
			"capabilities": map[string]bool{"experimentalApi": true},
		},
	}); err != nil {
		return nil, err
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
				return nil, fmt.Errorf("initialize: %s", response.Error.Message)
			}
			if err := encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
				return nil, err
			}
			// Like a background poll, skip the separate reset-credit lookup.
			if err := encoder.Encode(map[string]any{
				"method": "account/rateLimits/read", "id": 2,
				"params": map[string]bool{"excludeResetCreditDetails": true},
			}); err != nil {
				return nil, err
			}
		case 2:
			if response.Error != nil {
				return nil, fmt.Errorf("Codex could not report usage: %s", response.Error.Message)
			}
			return response.Result, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("Codex app-server closed before reporting usage")
}

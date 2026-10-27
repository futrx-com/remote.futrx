package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

// claudeUsageRequestID names the one control request a usage read sends.
const claudeUsageRequestID = "remote-plan-usage"

var _ agent.PlanUsageReader = (*Provider)(nil)

// ReadPlanUsage asks Claude Code for each account's plan limits through the
// get_usage control request its /usage screen is built from. The CLI fetches
// them from claude.ai and refreshes an expired sign-in as a run would. No
// prompt is sent, so a read spends none of the plan.
//
// Saved accounts are read one at a time in private config directories, except
// while a chat runs on one: the chat reports that account's limits itself,
// and a read could refresh the login the chat still uses. The host login is
// read only while no saved account is active, because only then do chats
// without a pinned account run on it, and account changes wait for that read.
func (p *Provider) ReadPlanUsage(ctx context.Context) []agent.AccountPlanUsage {
	var usages []agent.AccountPlanUsage
	readHost := true
	if p.accounts != nil {
		snapshot := p.accounts.AccountsSnapshot()
		for _, account := range snapshot.Items {
			p.accounts.WithIdleIsolatedAccount(account.ID, func() {
				usages = append(usages, p.readSavedAccountUsage(ctx, account.ID))
			})
		}
		readHost = snapshot.ActiveAccountID == ""
	}
	if readHost {
		readHostLogin := func() {
			if !hostHasSubscriptionLogin() {
				return
			}
			windows, err := readClaudeUsage(ctx, "")
			usages = append(usages, agent.AccountPlanUsage{Windows: windows, Err: err})
		}
		if p.accounts == nil {
			readHostLogin()
		} else {
			p.accounts.WithIdleHostLogin("", readHostLogin)
		}
	}
	return usages
}

// readSavedAccountUsage reads one saved account's plan in a private config
// directory, then offers a login the CLI refreshed to the account's vault
// record by the same rules as a run.
func (p *Provider) readSavedAccountUsage(ctx context.Context, accountID string) agent.AccountPlanUsage {
	usage := agent.AccountPlanUsage{AccountID: accountID}
	saved, _, err := p.accounts.CredentialForRun(accountID)
	if err != nil {
		usage.Err = err
		return usage
	}
	root, home, err := prepareIsolatedClaudeHome("remote-claude-usage-*")
	if err != nil {
		usage.Err = fmt.Errorf("prepare Claude usage read: %w", err)
		return usage
	}
	defer os.RemoveAll(root)
	credentialPath := filepath.Join(home, ".credentials.json")
	configPath := filepath.Join(home, ".claude.json")
	if err := writeAccountCredential(credentialPath, configPath, saved.Credential); err != nil {
		usage.Err = err
		return usage
	}

	usage.Windows, usage.Err = readClaudeUsage(ctx, home)
	if usage.Err == nil && len(usage.Windows) == 0 {
		usage.Err = errors.New("Claude did not report plan limits for this account")
	}

	if refreshed, err := readAccountCredential(credentialPath, configPath); err == nil {
		captureCtx, cancel := context.WithTimeout(context.Background(), p.credentialSyncTimeout)
		if err := p.accounts.CaptureRunCredential(captureCtx, saved, refreshed); err != nil {
			log.Printf("claude plan usage: keep refreshed login for account %s: %v", accountID, err)
		}
		cancel()
	}
	return usage
}

// hostHasSubscriptionLogin reports whether the host CLI is signed in to a
// Claude subscription, the only login with plan limits to read.
func hostHasSubscriptionLogin() bool {
	credentials, err := readJSONObject(claudeCredentialPath())
	return err == nil && len(credentials["claudeAiOauth"]) > 0
}

// readClaudeUsage runs the CLI headless against configDir, or the host's own
// config when configDir is empty, and returns the plan windows its get_usage
// answer reports. Closing the CLI's input after the answer ends it before any
// prompt.
func readClaudeUsage(ctx context.Context, configDir string) ([]agent.Quota, error) {
	ctx, cancel := context.WithTimeout(ctx, configconstants.PlanUsageAccountReadTimeout)
	defer cancel()

	cmd := exec.Command("claude",
		"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose")
	cmd.Env = claudeUsageEnv(os.Environ(), configDir)
	cmd.Dir = os.TempDir()
	var report claudeUsageReport
	stderr, err := agentruntime.Converse(ctx, cmd, configconstants.ClaudePlanUsageExitGrace, func(stdin io.Writer, stdout io.Reader) error {
		var err error
		report, err = requestClaudeUsage(stdin, stdout)
		return err
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("Claude usage read timed out: %w", ctx.Err())
		}
		if stderr != "" {
			return nil, fmt.Errorf("%w; claude: %s", err, stderr)
		}
		return nil, err
	}
	return report.windows(time.Now()), nil
}

// claudeUsageEnv points the CLI at configDir, or leaves the host's own
// config in place when configDir is empty. API credentials are dropped
// either way, so the CLI reports the subscription login's plan.
func claudeUsageEnv(base []string, configDir string) []string {
	if configDir != "" {
		return isolatedClaudeAuthEnvFor(base, configDir)
	}
	out := make([]string, 0, len(base))
	for _, env := range base {
		if strings.HasPrefix(env, "ANTHROPIC_API_KEY=") ||
			strings.HasPrefix(env, "ANTHROPIC_AUTH_TOKEN=") ||
			strings.HasPrefix(env, "CLAUDE_CODE_OAUTH_TOKEN=") {
			continue
		}
		out = append(out, env)
	}
	return out
}

// requestClaudeUsage sends the get_usage control request and waits for its
// answer, skipping any other line the CLI prints.
func requestClaudeUsage(stdin io.Writer, stdout io.Reader) (claudeUsageReport, error) {
	if err := json.NewEncoder(stdin).Encode(map[string]any{
		"type":       "control_request",
		"request_id": claudeUsageRequestID,
		"request":    map[string]any{"subtype": "get_usage", "skip_behaviors": true},
	}); err != nil {
		return claudeUsageReport{}, fmt.Errorf("send Claude usage request: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var line struct {
			Type     string `json:"type"`
			Response struct {
				Subtype   string          `json:"subtype"`
				RequestID string          `json:"request_id"`
				Error     string          `json:"error"`
				Response  json.RawMessage `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil || line.Type != "control_response" ||
			line.Response.RequestID != claudeUsageRequestID {
			continue
		}
		if line.Response.Subtype != "success" {
			message := strings.TrimSpace(line.Response.Error)
			if message == "" {
				message = "the request failed"
			}
			return claudeUsageReport{}, fmt.Errorf("Claude could not report usage: %s", message)
		}
		var report claudeUsageReport
		if err := json.Unmarshal(line.Response.Response, &report); err != nil {
			return claudeUsageReport{}, fmt.Errorf("decode Claude usage: %w", err)
		}
		return report, nil
	}
	if err := scanner.Err(); err != nil {
		return claudeUsageReport{}, err
	}
	return claudeUsageReport{}, errors.New("Claude exited before reporting usage")
}

// claudeUsageReport is the part of a get_usage answer that describes the
// plan: the same rows Claude Code's /usage shows as "Current session" and
// "Current week (all models)".
type claudeUsageReport struct {
	RateLimitsAvailable bool `json:"rate_limits_available"`
	RateLimits          *struct {
		FiveHour *claudeUsageWindow `json:"five_hour"`
		SevenDay *claudeUsageWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

type claudeUsageWindow struct {
	// Utilization is the percentage of the window used, 0-100.
	Utilization *float64 `json:"utilization"`
	// ResetsAt is an ISO 8601 timestamp.
	ResetsAt *string `json:"resets_at"`
}

func (r claudeUsageReport) windows(now time.Time) []agent.Quota {
	if !r.RateLimitsAvailable || r.RateLimits == nil {
		return nil
	}
	var windows []agent.Quota
	for _, pair := range []struct {
		window agent.QuotaWindow
		source *claudeUsageWindow
	}{
		{agent.QuotaWindowSession, r.RateLimits.FiveHour},
		{agent.QuotaWindowWeekly, r.RateLimits.SevenDay},
	} {
		// Like /usage, a window without a utilization is not shown at all.
		source := pair.source
		if source == nil || source.Utilization == nil ||
			math.IsNaN(*source.Utilization) || math.IsInf(*source.Utilization, 0) || *source.Utilization < 0 {
			continue
		}
		used := *source.Utilization
		quota := agent.Quota{Window: pair.window, UsedPercent: &used, MeasuredAt: now.UnixMilli()}
		if source.ResetsAt != nil {
			if resetsAt, err := time.Parse(time.RFC3339Nano, *source.ResetsAt); err == nil {
				quota.ResetsAt = resetsAt.Unix()
			}
		}
		windows = append(windows, quota)
	}
	return windows
}

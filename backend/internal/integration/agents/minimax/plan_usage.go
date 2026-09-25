package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

var _ agent.PlanUsageReader = (*Provider)(nil)

// tokenPlanReader is the provider's narrow dependency on MiniMax's Token Plan
// API. The module factory shares one concrete client with API-key validation.
type tokenPlanReader interface {
	tokenPlan(context.Context, string) ([]json.RawMessage, error)
}

// ReadPlanUsage asks MiniMax's Token Plan API for each saved key. An API key
// does not rotate during the read, so accounts can be read independently of
// running chats. Pay-as-you-go keys are rejected by the existing key service.
func (p *Provider) ReadPlanUsage(ctx context.Context) []agent.AccountPlanUsage {
	if p.apiKeys == nil {
		return nil
	}
	if accounts, ok := p.apiKeys.(interface {
		AccountsSnapshot() agentauth.AccountsSnapshot
		AccountsEnabled() bool
	}); ok && accounts.AccountsEnabled() {
		snapshot := accounts.AccountsSnapshot()
		usages := make([]agent.AccountPlanUsage, 0, len(snapshot.Items))
		for _, account := range snapshot.Items {
			usages = append(usages, p.readAccountPlanUsage(ctx, account.ID))
		}
		return usages
	}
	if _, ok := p.apiKeys.APIKey(); !ok {
		return nil
	}
	return []agent.AccountPlanUsage{p.readAccountPlanUsage(ctx, "")}
}

func (p *Provider) readAccountPlanUsage(ctx context.Context, accountID string) agent.AccountPlanUsage {
	usage := agent.AccountPlanUsage{AccountID: accountID}
	key, err := p.apiKey(accountID)
	if err != nil {
		usage.Err = err
		return usage
	}
	rows, err := p.planUsage.tokenPlan(ctx, key)
	if err != nil {
		usage.Err = err
		return usage
	}
	usage.Windows, usage.Err = miniMaxPlanWindows(rows, time.Now())
	return usage
}

// MiniMax's model_remains list can also contain image, speech, and video
// buckets. Only the general or MiniMax-M coding bucket describes this agent.
// The API's explicit remaining percentages avoid the ambiguous usage_count
// fields, which some Token Plans report as remaining counts.
func miniMaxPlanWindows(rows []json.RawMessage, now time.Time) ([]agent.Quota, error) {
	type modelRemain struct {
		ModelName                string   `json:"model_name"`
		IntervalRemainingPercent *float64 `json:"current_interval_remaining_percent"`
		WeeklyRemainingPercent   *float64 `json:"current_weekly_remaining_percent"`
		EndTime                  int64    `json:"end_time"`
		WeeklyEndTime            int64    `json:"weekly_end_time"`
		RemainsTime              int64    `json:"remains_time"`
		WeeklyRemainsTime        int64    `json:"weekly_remains_time"`
	}
	var selected *modelRemain
	for _, raw := range rows {
		var row modelRemain
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(row.ModelName))
		if name == "general" && (row.IntervalRemainingPercent != nil || row.WeeklyRemainingPercent != nil) {
			selected = &row
			break
		}
		if selected == nil && strings.HasPrefix(name, "minimax-m") {
			selected = &row
		}
	}
	if selected == nil {
		return nil, errors.New("MiniMax did not report a coding Token Plan limit")
	}
	measured := now.UnixMilli()
	var windows []agent.Quota
	appendWindow := func(kind agent.QuotaWindow, remaining *float64, endMillis, remainingMillis int64) {
		if remaining == nil || math.IsNaN(*remaining) || math.IsInf(*remaining, 0) ||
			*remaining < 0 || *remaining > 100 {
			return
		}
		used := 100 - *remaining
		reset := endMillis / 1000
		if reset <= 0 && remainingMillis > 0 {
			reset = now.Add(time.Duration(remainingMillis) * time.Millisecond).Unix()
		}
		windows = append(windows, agent.Quota{
			Window: kind, UsedPercent: &used, ResetsAt: reset, MeasuredAt: measured,
		})
	}
	appendWindow(agent.QuotaWindowSession, selected.IntervalRemainingPercent, selected.EndTime, selected.RemainsTime)
	appendWindow(agent.QuotaWindowWeekly, selected.WeeklyRemainingPercent, selected.WeeklyEndTime, selected.WeeklyRemainsTime)
	if len(windows) == 0 {
		return nil, errors.New("MiniMax did not report a percentage for its coding Token Plan")
	}
	return windows, nil
}

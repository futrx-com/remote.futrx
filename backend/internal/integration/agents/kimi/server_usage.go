package kimi

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// runUsage owns disjoint per-step accounting across main and delegated agents.
// Cumulative child completion totals must not be added a second time.
type runUsage struct {
	started time.Time
	steps   map[string]bool
	totals  agent.Usage
}

func newRunUsage(model string) runUsage {
	return runUsage{started: time.Now(), steps: map[string]bool{}, totals: agent.Usage{Model: model}}
}
func (u *runUsage) setModel(model string) { u.totals.Model = model }
func (u *runUsage) startTurn()            { u.totals.Turns++ }
func (u *runUsage) recordStep(who string, p nativePayload) bool {
	key := fmt.Sprintf("%s:%d:%d:%s", who, p.TurnID, p.Step, p.StepID)
	if p.Usage == nil || u.steps[key] {
		return false
	}
	u.steps[key] = true
	u.totals.InputTokens += p.Usage.InputOther
	u.totals.OutputTokens += p.Usage.Output
	u.totals.CacheReadTokens += p.Usage.InputCacheRead
	u.totals.CacheWriteTokens += p.Usage.InputCacheCreation
	return true
}
func (u *runUsage) raw() json.RawMessage {
	u.totals.DurationMs = time.Since(u.started).Milliseconds()
	return u.totals.Raw()
}

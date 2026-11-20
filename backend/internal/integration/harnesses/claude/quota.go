package claude

import (
	"encoding/json"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

// rateLimitInfo is the payload of a rate_limit_event line.
type rateLimitInfo struct {
	Status        string   `json:"status"`
	ResetsAt      int64    `json:"resetsAt"`
	RateLimitType string   `json:"rateLimitType"`
	Utilization   *float64 `json:"utilization"`
}

// parseRateLimit converts one line into a platform quota reading, or reports
// that there was nothing usable in it.
//
// An unknown window name is dropped rather than guessed at: filing a reading
// under the wrong window would make the card confidently wrong, which is worse
// than one that has not heard about that window yet.
func parseRateLimit(payload json.RawMessage, now int64) (harness.Quota, bool) {
	if len(payload) == 0 {
		return harness.Quota{}, false
	}
	var info rateLimitInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return harness.Quota{}, false
	}
	window := normalizeQuotaWindow(info.RateLimitType)
	if window == "" {
		return harness.Quota{}, false
	}
	quota := harness.Quota{
		Window:     window,
		ResetsAt:   info.ResetsAt,
		Status:     info.Status,
		MeasuredAt: now,
	}
	// Utilization is a fraction when the CLI sends it at all; the card wants a
	// percentage, and an absent value stays absent.
	if info.Utilization != nil {
		percent := *info.Utilization
		if percent <= 1 {
			percent *= 100
		}
		quota.UsedPercent = &percent
	}
	return quota, true
}

// normalizeQuotaWindow keeps Claude's wire vocabulary inside its adapter.
// Unknown names stay unknown so the caller drops them instead of guessing.
func normalizeQuotaWindow(name string) harness.QuotaWindow {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "five_hour", "5h", "primary", "session":
		return harness.QuotaWindowSession
	case "seven_day", "weekly", "7d", "secondary":
		return harness.QuotaWindowWeekly
	default:
		return ""
	}
}

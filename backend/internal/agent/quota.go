package agent

import "strings"

// Subscription quota, as the agent CLIs report it.
//
// This is the operator's *plan* allowance — the rolling window a Claude Max or
// ChatGPT subscription resets on — and it is a different number from anything
// the platform can compute itself. The usage ledger knows what this platform
// spent; the plan is spent from everywhere the operator works, including their
// laptop, so only the vendor can say how much is left.
//
// Both CLIs volunteer it mid-run and neither offers a way to ask:
//
//   - claude emits a top-level {"type":"rate_limit_event"} line carrying one
//     window at a time — "five_hour" or "seven_day" — with an absolute reset
//     time and a status.
//   - codex reports rate_limits inside its token_count event, as a primary and
//     a secondary window, each with a percentage used and a window length in
//     minutes.
//
// Neither is a request this platform can make, so a window is only as fresh as
// the last run that touched it. Anything built on this must say when it was
// measured rather than implying it is live.

// QuotaWindow names a rolling allowance. The two CLIs use different vocabulary
// for the same two shapes, and this is the platform's.
type QuotaWindow string

const (
	// QuotaWindowSession is the short rolling window — five hours on both
	// Claude and ChatGPT subscriptions today.
	QuotaWindowSession QuotaWindow = "session"
	// QuotaWindowWeekly is the long one.
	QuotaWindowWeekly QuotaWindow = "weekly"
)

// Quota is one window's state at one moment.
type Quota struct {
	Window QuotaWindow `json:"window"`
	// UsedPercent is 0–100 where the CLI reports it, and nil where it does
	// not. Claude reports a status rather than a number, so a Claude window
	// usually has a reset time and no percentage: absent is not zero.
	UsedPercent *float64 `json:"usedPercent,omitempty"`
	// ResetsAt is a Unix second. Zero means the CLI did not say.
	ResetsAt int64 `json:"resetsAt,omitempty"`
	// Status is the CLI's own word — "allowed", "allowed_warning",
	// "rejected". Passed through rather than mapped, because a vendor adding
	// a state should show up as that state and not as a wrong guess.
	Status string `json:"status,omitempty"`
	// MeasuredAt is when this platform saw it, in Unix milliseconds. It is
	// the honest half of the reading: the rest is a snapshot from whenever
	// the last run happened.
	MeasuredAt int64 `json:"measuredAt"`
}

// NormalizeQuotaWindow maps a CLI's own name onto the platform's two. An
// unrecognized name returns "" so a caller drops the reading rather than
// filing it under the wrong window.
func NormalizeQuotaWindow(name string) QuotaWindow {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "five_hour", "5h", "primary", "session":
		return QuotaWindowSession
	case "seven_day", "weekly", "7d", "secondary":
		return QuotaWindowWeekly
	default:
		return ""
	}
}

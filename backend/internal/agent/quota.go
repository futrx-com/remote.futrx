package agent

// Subscription quota, as the agent CLIs report it.
//
// This is the operator's *plan* allowance — the rolling window a Claude Max or
// ChatGPT subscription resets on — and it is a different number from anything
// the platform can compute itself. The usage ledger knows what this platform
// spent; the plan is spent from everywhere the operator works, including their
// laptop, so only the vendor can say how much is left.
//
// Provider integrations expose it through their native protocols:
//
//   - claude emits a top-level {"type":"rate_limit_event"} line carrying one
//     window at a time — "five_hour" or "seven_day" — with an absolute reset
//     time and a status.
//   - codex app-server describes primary and secondary rate-limit windows,
//     including percentage used, reset time, and window length.
//
// The normalized value is a last-seen observation, not a live counter.
// Anything built on it must say when it was measured rather than implying it
// is current.

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

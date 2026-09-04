// Package usage owns the token and cost ledger: one append-only record per
// completed agent run, the price table used to estimate cost when a provider
// does not report one, and the aggregations the Usage page renders.
package usage

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrForbidden   = errors.New("not allowed to read this project's usage")
	ErrUnavailable = errors.New("usage ledger is unavailable")
	ErrInvalidTime = errors.New("invalid time range")
)

// Record is one completed agent run. Records are append-only; the file they
// live in is chosen by the UTC month of At.
type Record struct {
	At          int64  `json:"at"`
	ProjectID   string `json:"projectId,omitempty"`
	ProjectSlug string `json:"projectSlug,omitempty"`
	ChatID      string `json:"chatId"`
	RunID       string `json:"runId,omitempty"`
	UserEmail   string `json:"userEmail,omitempty"`
	Provider    string `json:"provider"`
	Model       string `json:"model,omitempty"`

	// InputTokens is uncached input; all four token fields are disjoint.
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`

	// CostUSD is nil when the cost of the run is unknown: the provider did
	// not price it and no price-table entry matched its model.
	CostUSD *float64 `json:"costUsd,omitempty"`
	// Estimated marks a CostUSD that came from the price table rather than
	// from the provider.
	Estimated bool `json:"estimated,omitempty"`

	DurationMs int64 `json:"durationMs,omitempty"`
	Turns      int64 `json:"turns,omitempty"`
	Scheduled  bool  `json:"scheduled,omitempty"`
}

// TotalTokens is the billable token count across every bucket.
func (r Record) TotalTokens() int64 {
	return r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheWriteTokens
}

// GroupBy names the dimension a summary is bucketed by.
type GroupBy string

const (
	GroupByProject  GroupBy = "project"
	GroupByUser     GroupBy = "user"
	GroupByProvider GroupBy = "provider"
	GroupByModel    GroupBy = "model"
	GroupByDay      GroupBy = "day"
	GroupByChat     GroupBy = "chat"
)

// NormalizeGroupBy collapses unknown values onto the project view, which is
// the one the Usage page opens with.
func NormalizeGroupBy(value string) GroupBy {
	switch GroupBy(strings.ToLower(strings.TrimSpace(value))) {
	case GroupByUser:
		return GroupByUser
	case GroupByProvider:
		return GroupByProvider
	case GroupByModel:
		return GroupByModel
	case GroupByDay:
		return GroupByDay
	case GroupByChat:
		return GroupByChat
	default:
		return GroupByProject
	}
}

// Totals is the aggregate of a set of records.
type Totals struct {
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
	TotalTokens      int64 `json:"totalTokens"`
	// CostUSD sums every record whose cost is known, exact or estimated.
	CostUSD float64 `json:"costUsd"`
	// EstimatedCostUSD is the part of CostUSD that came from the price table.
	EstimatedCostUSD float64 `json:"estimatedCostUsd"`
	Runs             int64   `json:"runs"`
	// UnpricedRuns counts runs whose cost could not be determined at all.
	UnpricedRuns int64 `json:"unpricedRuns"`
}

func (t *Totals) add(record Record) {
	t.InputTokens += record.InputTokens
	t.OutputTokens += record.OutputTokens
	t.CacheReadTokens += record.CacheReadTokens
	t.CacheWriteTokens += record.CacheWriteTokens
	t.TotalTokens += record.TotalTokens()
	t.Runs++
	if record.CostUSD == nil {
		t.UnpricedRuns++
		return
	}
	t.CostUSD += *record.CostUSD
	if record.Estimated {
		t.EstimatedCostUSD += *record.CostUSD
	}
}

// Group is one bucket of a summary. Totals is embedded, so its fields are
// promoted into the group object on the wire.
type Group struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Totals
}

// DayPoint is one bar of the per-day chart. Day is an ISO date in UTC.
type DayPoint struct {
	Day         string  `json:"day"`
	TotalTokens int64   `json:"totalTokens"`
	CostUSD     float64 `json:"costUsd"`
	Runs        int64   `json:"runs"`
}

// Summary is the response of the summary endpoint: overall totals, one
// bucket per group, and the per-day series the chart draws.
type Summary struct {
	From     int64      `json:"from"`
	To       int64      `json:"to"`
	GroupBy  GroupBy    `json:"groupBy"`
	Totals   Totals     `json:"totals"`
	Projects int        `json:"projects"`
	Groups   []Group    `json:"groups"`
	Daily    []DayPoint `json:"daily"`
}

// Query selects the records an aggregation reads.
type Query struct {
	From      int64
	To        int64
	GroupBy   GroupBy
	ProjectID string
	ChatID    string
}

// RecordQuery pages the raw ledger, newest first.
type RecordQuery struct {
	From      int64
	To        int64
	ProjectID string
	ChatID    string
	Limit     int
	Cursor    string
}

// RecordPage is one page of raw records, newest first.
type RecordPage struct {
	Records    []Record `json:"records"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// RebuildResult reports what a ledger rebuild produced.
type RebuildResult struct {
	Chats           int      `json:"chats"`
	Records         int      `json:"records"`
	Months          []string `json:"months"`
	PreservedActors int      `json:"preservedActors"`
}

// dayKey renders a UTC millisecond timestamp as the ISO date used for
// per-day buckets.
func dayKey(at int64) string {
	return time.UnixMilli(at).UTC().Format("2006-01-02")
}

// MonthKey renders the YYYY-MM file suffix a record belongs to.
func MonthKey(at int64) string {
	return time.UnixMilli(at).UTC().Format("2006-01")
}

package usage

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const (
	defaultRecordLimit = 100
	maxRecordLimit     = 500
)

// RunEvent is what the prompt service hands over when an agent run ends. It
// carries the raw provider usage blob so the ledger, not the prompt service,
// owns the token/cost vocabulary.
type RunEvent struct {
	At        int64
	ChatID    string
	ProjectID string
	RunID     string
	UserEmail string
	Provider  string
	Model     string
	Usage     json.RawMessage
	Scheduled bool
}

// Service turns completed runs into ledger records and answers aggregation
// queries over them.
type Service struct {
	repo     Repository
	projects ProjectDirectory
	chats    ChatDirectory
}

func New(repo Repository, projects ProjectDirectory, chats ChatDirectory) *Service {
	return &Service{repo: repo, projects: projects, chats: chats}
}

// RecordRun appends one completed run to the ledger. It never fails a run:
// persistence problems are logged and swallowed, because losing a usage line
// must not lose the user's turn.
func (s *Service) RecordRun(ctx context.Context, event RunEvent) {
	if s == nil || s.repo == nil {
		return
	}
	record, ok := s.recordFromRun(ctx, event)
	if !ok {
		return
	}
	if err := s.repo.Append(ctx, record); err != nil {
		log.Printf("usage: append record for chat %s: %v", record.ChatID, err)
	}
}

func (s *Service) recordFromRun(ctx context.Context, event RunEvent) (Record, bool) {
	if strings.TrimSpace(event.ChatID) == "" {
		return Record{}, false
	}
	at := event.At
	if at <= 0 {
		at = time.Now().UnixMilli()
	}
	usage, _ := agent.ParseUsage(event.Usage)

	model := strings.TrimSpace(usage.Model)
	if model == "" {
		model = strings.TrimSpace(event.Model)
	}
	record := Record{
		At:               at,
		ProjectID:        strings.TrimSpace(event.ProjectID),
		ChatID:           strings.TrimSpace(event.ChatID),
		RunID:            strings.TrimSpace(event.RunID),
		UserEmail:        strings.ToLower(strings.TrimSpace(event.UserEmail)),
		Provider:         strings.TrimSpace(event.Provider),
		Model:            model,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		DurationMs:       usage.DurationMs,
		Turns:            usage.Turns,
		Scheduled:        event.Scheduled,
	}
	record.ProjectSlug = s.projectSlug(ctx, record.ProjectID)
	s.applyCost(ctx, &record, usage.CostUSD)
	return record, true
}

// applyCost prefers the provider's own price and only estimates when there
// is none. An unknown cost stays nil rather than collapsing to zero.
func (s *Service) applyCost(ctx context.Context, record *Record, providerCost *float64) {
	if providerCost != nil {
		cost := *providerCost
		record.CostUSD = &cost
		record.Estimated = false
		return
	}
	prices, err := s.repo.Prices(ctx)
	if err != nil {
		log.Printf("usage: read price table: %v", err)
		return
	}
	estimate, ok := prices.Estimate(
		record.Model,
		record.InputTokens,
		record.OutputTokens,
		record.CacheReadTokens,
		record.CacheWriteTokens,
	)
	if !ok {
		return
	}
	record.CostUSD = &estimate
	record.Estimated = true
}

func (s *Service) projectSlug(ctx context.Context, projectID string) string {
	if projectID == "" || s.projects == nil {
		return ""
	}
	meta, err := s.projects.Get(ctx, serviceproject.ID(projectID))
	if err != nil {
		return ""
	}
	return meta.Slug
}

// Prices returns the editable price table.
func (s *Service) Prices(ctx context.Context) (PriceTable, error) {
	if s == nil || s.repo == nil {
		return PriceTable{}, ErrUnavailable
	}
	return s.repo.Prices(ctx)
}

// SetPrices replaces the editable price table after normalizing it.
func (s *Service) SetPrices(ctx context.Context, table PriceTable) (PriceTable, error) {
	if s == nil || s.repo == nil {
		return PriceTable{}, ErrUnavailable
	}
	normalized, err := table.Normalize()
	if err != nil {
		return PriceTable{}, err
	}
	normalized.UpdatedAt = time.Now().UnixMilli()
	return s.repo.SetPrices(ctx, normalized)
}

// Summary aggregates the caller's visible records over a time window.
func (s *Service) Summary(
	ctx context.Context,
	query Query,
	callerEmail string,
	isAdmin bool,
) (Summary, error) {
	if s == nil || s.repo == nil {
		return Summary{}, ErrUnavailable
	}
	from, to, err := normalizeWindow(query.From, query.To)
	if err != nil {
		return Summary{}, err
	}
	scope, err := s.scopeFor(ctx, callerEmail, isAdmin)
	if err != nil {
		return Summary{}, err
	}
	if query.ProjectID != "" && !scope.allowsProject(query.ProjectID) {
		return Summary{}, ErrForbidden
	}

	groupBy := query.GroupBy
	if groupBy == "" {
		groupBy = GroupByProject
	}
	summary := Summary{From: from, To: to, GroupBy: groupBy}
	groups := map[string]*Group{}
	days := map[string]*DayPoint{}
	projects := map[string]struct{}{}

	err = s.repo.Scan(ctx, from, to, func(record Record) bool {
		if !scope.allows(record) || !matchesFilter(record, query.ProjectID, query.ChatID) {
			return true
		}
		summary.Totals.add(record)
		if record.ProjectID != "" {
			projects[record.ProjectID] = struct{}{}
		}

		key, label := groupKey(record, groupBy)
		group, ok := groups[key]
		if !ok {
			group = &Group{Key: key, Label: label}
			groups[key] = group
		}
		group.Totals.add(record)

		day := dayKey(record.At)
		point, ok := days[day]
		if !ok {
			point = &DayPoint{Day: day}
			days[day] = point
		}
		point.TotalTokens += record.TotalTokens()
		point.Runs++
		if record.CostUSD != nil {
			point.CostUSD += *record.CostUSD
		}
		return true
	})
	if err != nil {
		return Summary{}, err
	}

	summary.Projects = len(projects)
	summary.Groups = sortedGroups(groups)
	summary.Daily = fillDays(days, from, to)
	return summary, nil
}

// ProjectSummary is the per-project quick total used by the project page.
func (s *Service) ProjectSummary(
	ctx context.Context,
	projectID string,
	from, to int64,
	callerEmail string,
	isAdmin bool,
) (Summary, error) {
	return s.Summary(ctx, Query{
		From:      from,
		To:        to,
		GroupBy:   GroupByModel,
		ProjectID: projectID,
	}, callerEmail, isAdmin)
}

// Records pages the raw ledger newest-first. The cursor is the number of
// records already returned within the same window and filter.
func (s *Service) Records(
	ctx context.Context,
	query RecordQuery,
	callerEmail string,
	isAdmin bool,
) (RecordPage, error) {
	if s == nil || s.repo == nil {
		return RecordPage{}, ErrUnavailable
	}
	from, to, err := normalizeWindow(query.From, query.To)
	if err != nil {
		return RecordPage{}, err
	}
	scope, err := s.scopeFor(ctx, callerEmail, isAdmin)
	if err != nil {
		return RecordPage{}, err
	}
	if query.ProjectID != "" && !scope.allowsProject(query.ProjectID) {
		return RecordPage{}, ErrForbidden
	}

	limit := query.Limit
	if limit <= 0 {
		limit = defaultRecordLimit
	}
	if limit > maxRecordLimit {
		limit = maxRecordLimit
	}
	offset := 0
	if query.Cursor != "" {
		parsed, convErr := strconv.Atoi(query.Cursor)
		if convErr != nil || parsed < 0 {
			return RecordPage{}, ErrInvalidTime
		}
		offset = parsed
	}

	matched := make([]Record, 0, limit)
	err = s.repo.Scan(ctx, from, to, func(record Record) bool {
		if !scope.allows(record) || !matchesFilter(record, query.ProjectID, query.ChatID) {
			return true
		}
		matched = append(matched, record)
		return true
	})
	if err != nil {
		return RecordPage{}, err
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].At > matched[j].At })

	if offset >= len(matched) {
		return RecordPage{Records: []Record{}}, nil
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	page := RecordPage{Records: matched[offset:end]}
	if end < len(matched) {
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}

// scope is the per-caller visibility filter: which projects they may read and
// whether they may read project-less ("loose") chat runs from other users.
type scope struct {
	isAdmin  bool
	email    string
	projects map[string]struct{}
}

func (s scope) allowsProject(projectID string) bool {
	if s.isAdmin {
		return true
	}
	if projectID == "" {
		return true
	}
	_, ok := s.projects[projectID]
	return ok
}

func (s scope) allows(record Record) bool {
	if s.isAdmin {
		return true
	}
	if record.ProjectID == "" {
		// Loose chats have no membership list, so a member only sees the
		// runs attributed to their own account.
		return s.email != "" && record.UserEmail == s.email
	}
	_, ok := s.projects[record.ProjectID]
	return ok
}

func (s *Service) scopeFor(ctx context.Context, callerEmail string, isAdmin bool) (scope, error) {
	email := strings.ToLower(strings.TrimSpace(callerEmail))
	if isAdmin {
		return scope{isAdmin: true, email: email}, nil
	}
	out := scope{email: email, projects: map[string]struct{}{}}
	if s.projects == nil {
		return out, nil
	}
	visible, err := s.projects.ListVisible(ctx, email, false)
	if err != nil {
		return scope{}, err
	}
	for _, meta := range visible {
		out.projects[string(meta.ID)] = struct{}{}
	}
	return out, nil
}

func matchesFilter(record Record, projectID, chatID string) bool {
	if projectID != "" && record.ProjectID != projectID {
		return false
	}
	if chatID != "" && record.ChatID != chatID {
		return false
	}
	return true
}

func groupKey(record Record, groupBy GroupBy) (string, string) {
	switch groupBy {
	case GroupByUser:
		if record.UserEmail == "" {
			return "", "Unattributed"
		}
		return record.UserEmail, record.UserEmail
	case GroupByProvider:
		if record.Provider == "" {
			return "", "Unknown provider"
		}
		return record.Provider, record.Provider
	case GroupByModel:
		if record.Model == "" {
			return "", "Unknown model"
		}
		return record.Model, record.Model
	case GroupByDay:
		day := dayKey(record.At)
		return day, day
	case GroupByChat:
		return record.ChatID, record.ChatID
	default:
		if record.ProjectID == "" {
			return "", "No project"
		}
		label := record.ProjectSlug
		if label == "" {
			label = record.ProjectID
		}
		return record.ProjectID, label
	}
}

func sortedGroups(groups map[string]*Group) []Group {
	out := make([]Group, 0, len(groups))
	for _, group := range groups {
		out = append(out, *group)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CostUSD != out[j].CostUSD {
			return out[i].CostUSD > out[j].CostUSD
		}
		if out[i].TotalTokens != out[j].TotalTokens {
			return out[i].TotalTokens > out[j].TotalTokens
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// fillDays returns one point per UTC day in the window so the chart has a
// continuous x-axis even on days with no runs.
func fillDays(days map[string]*DayPoint, from, to int64) []DayPoint {
	const maxDays = 400
	start := time.UnixMilli(from).UTC().Truncate(24 * time.Hour)
	end := time.UnixMilli(to).UTC().Truncate(24 * time.Hour)
	out := make([]DayPoint, 0, len(days)+1)
	for day, count := start, 0; !day.After(end) && count < maxDays; day, count = day.AddDate(0, 0, 1), count+1 {
		key := day.Format("2006-01-02")
		if point, ok := days[key]; ok {
			out = append(out, *point)
			continue
		}
		out = append(out, DayPoint{Day: key})
	}
	return out
}

// normalizeWindow defaults an open range to the last 30 days and rejects an
// inverted one.
func normalizeWindow(from, to int64) (int64, int64, error) {
	if to <= 0 {
		to = time.Now().UnixMilli()
	}
	if from <= 0 {
		from = time.UnixMilli(to).UTC().AddDate(0, 0, -30).UnixMilli()
	}
	if from > to {
		return 0, 0, ErrInvalidTime
	}
	return from, to, nil
}

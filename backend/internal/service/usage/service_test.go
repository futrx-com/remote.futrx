package usage

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"testing"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// fakeRepository is an in-memory ledger. Aggregation math is independent of
// the file layout, which fileusage tests separately.
type fakeRepository struct {
	records []Record
	prices  PriceTable
	written [][]Record
}

func newFakeRepository(records ...Record) *fakeRepository {
	table, err := DefaultPriceTable().Normalize()
	if err != nil {
		panic(err)
	}
	return &fakeRepository{records: records, prices: table}
}

func (r *fakeRepository) Append(_ context.Context, record Record) error {
	r.records = append(r.records, record)
	return nil
}

func (r *fakeRepository) Scan(_ context.Context, from, to int64, visit func(Record) bool) error {
	ordered := append([]Record(nil), r.records...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].At < ordered[j].At })
	for _, record := range ordered {
		if from > 0 && record.At < from {
			continue
		}
		if to > 0 && record.At > to {
			continue
		}
		if !visit(record) {
			return nil
		}
	}
	return nil
}

func (r *fakeRepository) ReplaceAll(_ context.Context, records []Record) ([]string, error) {
	r.written = append(r.written, append([]Record(nil), records...))
	r.records = append([]Record(nil), records...)
	months := map[string]struct{}{}
	for _, record := range records {
		months[MonthKey(record.At)] = struct{}{}
	}
	out := make([]string, 0, len(months))
	for month := range months {
		out = append(out, month)
	}
	sort.Strings(out)
	return out, nil
}

func (r *fakeRepository) Prices(context.Context) (PriceTable, error) { return r.prices, nil }

func (r *fakeRepository) SetPrices(_ context.Context, table PriceTable) (PriceTable, error) {
	r.prices = table
	return table, nil
}

type fakeProjects struct {
	metas   []serviceproject.Meta
	visible map[string][]string
}

func (p fakeProjects) Get(_ context.Context, id serviceproject.ID) (serviceproject.Meta, error) {
	for _, meta := range p.metas {
		if meta.ID == id {
			return meta, nil
		}
	}
	return serviceproject.Meta{}, serviceproject.ErrNotFound
}

func (p fakeProjects) ListVisible(
	_ context.Context,
	email string,
	isAdmin bool,
) ([]serviceproject.Meta, error) {
	if isAdmin {
		return p.metas, nil
	}
	allowed := p.visible[email]
	out := make([]serviceproject.Meta, 0, len(allowed))
	for _, meta := range p.metas {
		for _, id := range allowed {
			if string(meta.ID) == id {
				out = append(out, meta)
			}
		}
	}
	return out, nil
}

type fakeChats struct {
	metas  []servicechat.Meta
	events map[servicechat.ID][]servicechat.Event
}

func (c fakeChats) List(context.Context) ([]servicechat.Meta, error) { return c.metas, nil }

func (c fakeChats) ReadEvents(
	_ context.Context,
	id servicechat.ID,
) ([]servicechat.Event, error) {
	return c.events[id], nil
}

func ms(day int, hour int) int64 {
	return time.Date(2026, time.August, day, hour, 0, 0, 0, time.UTC).UnixMilli()
}

func cost(value float64) *float64 { return &value }

func testProjects() fakeProjects {
	return fakeProjects{
		metas: []serviceproject.Meta{
			{ID: "aaaa1111", Name: "Alpha", Slug: "alpha"},
			{ID: "bbbb2222", Name: "Beta", Slug: "beta"},
		},
		visible: map[string][]string{"member@example.com": {"aaaa1111"}},
	}
}

func testLedger() *fakeRepository {
	return newFakeRepository(
		Record{At: ms(1, 9), ProjectID: "aaaa1111", ProjectSlug: "alpha", ChatID: "c1", UserEmail: "member@example.com", Provider: "claude", Model: "claude-sonnet-4-5", InputTokens: 100, OutputTokens: 200, CostUSD: cost(0.5)},
		Record{At: ms(2, 9), ProjectID: "aaaa1111", ProjectSlug: "alpha", ChatID: "c1", UserEmail: "admin@example.com", Provider: "codex", Model: "gpt-5-codex", InputTokens: 10, OutputTokens: 20, CacheReadTokens: 5, CostUSD: cost(0.25), Estimated: true},
		Record{At: ms(3, 9), ProjectID: "bbbb2222", ProjectSlug: "beta", ChatID: "c2", UserEmail: "admin@example.com", Provider: "claude", Model: "claude-opus-4-5", InputTokens: 1, OutputTokens: 2, CostUSD: cost(1.25)},
		Record{At: ms(3, 10), ChatID: "c3", UserEmail: "member@example.com", Provider: "kimi", Model: "kimi-k2"},
	)
}

func TestSummaryAggregatesTotalsAndGroups(t *testing.T) {
	service := New(testLedger(), testProjects(), nil)
	summary, err := service.Summary(
		context.Background(),
		Query{From: ms(1, 0), To: ms(4, 0), GroupBy: GroupByProject},
		"admin@example.com",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	if summary.Totals.Runs != 4 {
		t.Fatalf("runs = %d, want 4", summary.Totals.Runs)
	}
	if summary.Totals.TotalTokens != 338 {
		t.Fatalf("tokens = %d, want 338", summary.Totals.TotalTokens)
	}
	if math.Abs(summary.Totals.CostUSD-2.0) > 1e-9 {
		t.Fatalf("cost = %v, want 2.0", summary.Totals.CostUSD)
	}
	if math.Abs(summary.Totals.EstimatedCostUSD-0.25) > 1e-9 {
		t.Fatalf("estimated cost = %v, want 0.25", summary.Totals.EstimatedCostUSD)
	}
	if summary.Totals.UnpricedRuns != 1 {
		t.Fatalf("unpriced runs = %d, want 1", summary.Totals.UnpricedRuns)
	}
	if summary.Projects != 2 {
		t.Fatalf("active projects = %d, want 2", summary.Projects)
	}

	if len(summary.Groups) != 3 {
		t.Fatalf("groups = %d, want 3 (two projects plus loose chats)", len(summary.Groups))
	}
	// Groups are ordered by cost, so Beta ($1.25) leads Alpha ($0.75).
	if summary.Groups[0].Key != "bbbb2222" || summary.Groups[0].Label != "beta" {
		t.Fatalf("first group = %+v", summary.Groups[0])
	}
	if summary.Groups[1].Key != "aaaa1111" || summary.Groups[1].Runs != 2 {
		t.Fatalf("second group = %+v", summary.Groups[1])
	}
	if summary.Groups[2].Key != "" || summary.Groups[2].Label != "No project" {
		t.Fatalf("loose-chat group = %+v", summary.Groups[2])
	}
}

func TestSummaryGroupByDimensions(t *testing.T) {
	service := New(testLedger(), testProjects(), nil)
	tests := []struct {
		groupBy GroupBy
		want    map[string]int64
	}{
		{groupBy: GroupByProvider, want: map[string]int64{"claude": 2, "codex": 1, "kimi": 1}},
		{
			groupBy: GroupByModel,
			want:    map[string]int64{"claude-sonnet-4-5": 1, "gpt-5-codex": 1, "claude-opus-4-5": 1, "kimi-k2": 1},
		},
		{groupBy: GroupByUser, want: map[string]int64{"member@example.com": 2, "admin@example.com": 2}},
		{groupBy: GroupByDay, want: map[string]int64{"2026-08-01": 1, "2026-08-02": 1, "2026-08-03": 2}},
		{groupBy: GroupByChat, want: map[string]int64{"c1": 2, "c2": 1, "c3": 1}},
	}

	for _, test := range tests {
		t.Run(string(test.groupBy), func(t *testing.T) {
			summary, err := service.Summary(
				context.Background(),
				Query{From: ms(1, 0), To: ms(4, 0), GroupBy: test.groupBy},
				"admin@example.com",
				true,
			)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]int64{}
			for _, group := range summary.Groups {
				got[group.Key] = group.Runs
			}
			if len(got) != len(test.want) {
				t.Fatalf("groups = %v, want %v", got, test.want)
			}
			for key, runs := range test.want {
				if got[key] != runs {
					t.Fatalf("group %q runs = %d, want %d (all: %v)", key, got[key], runs, got)
				}
			}
		})
	}
}

func TestSummaryDailySeriesFillsEmptyDays(t *testing.T) {
	service := New(testLedger(), testProjects(), nil)
	summary, err := service.Summary(
		context.Background(),
		Query{From: ms(1, 0), To: ms(5, 0)},
		"admin@example.com",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantDays := []string{"2026-08-01", "2026-08-02", "2026-08-03", "2026-08-04", "2026-08-05"}
	if len(summary.Daily) != len(wantDays) {
		t.Fatalf("daily points = %d, want %d", len(summary.Daily), len(wantDays))
	}
	for i, day := range wantDays {
		if summary.Daily[i].Day != day {
			t.Fatalf("daily[%d].Day = %q, want %q", i, summary.Daily[i].Day, day)
		}
	}
	if summary.Daily[2].Runs != 2 {
		t.Fatalf("Aug 3 runs = %d, want 2", summary.Daily[2].Runs)
	}
	if summary.Daily[3].Runs != 0 || summary.Daily[3].TotalTokens != 0 {
		t.Fatalf("Aug 4 should be an empty bar, got %+v", summary.Daily[3])
	}
}

func TestSummaryFiltersByMembership(t *testing.T) {
	service := New(testLedger(), testProjects(), nil)
	summary, err := service.Summary(
		context.Background(),
		Query{From: ms(1, 0), To: ms(4, 0), GroupBy: GroupByProject},
		"member@example.com",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	// The member belongs to Alpha only, and additionally sees the loose-chat
	// run attributed to their own account.
	if summary.Totals.Runs != 3 {
		t.Fatalf("runs = %d, want 3", summary.Totals.Runs)
	}
	for _, group := range summary.Groups {
		if group.Key == "bbbb2222" {
			t.Fatalf("member must not see project Beta: %+v", summary.Groups)
		}
	}
	if math.Abs(summary.Totals.CostUSD-0.75) > 1e-9 {
		t.Fatalf("cost = %v, want 0.75", summary.Totals.CostUSD)
	}
}

func TestSummaryRejectsForeignProjectFilter(t *testing.T) {
	service := New(testLedger(), testProjects(), nil)
	_, err := service.Summary(
		context.Background(),
		Query{From: ms(1, 0), To: ms(4, 0), ProjectID: "bbbb2222"},
		"member@example.com",
		false,
	)
	if err != ErrForbidden {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestSummaryRejectsInvertedWindow(t *testing.T) {
	service := New(testLedger(), testProjects(), nil)
	if _, err := service.Summary(
		context.Background(),
		Query{From: ms(4, 0), To: ms(1, 0)},
		"admin@example.com",
		true,
	); err != ErrInvalidTime {
		t.Fatalf("err = %v, want ErrInvalidTime", err)
	}
}

func TestRecordsPageNewestFirst(t *testing.T) {
	service := New(testLedger(), testProjects(), nil)
	first, err := service.Records(
		context.Background(),
		RecordQuery{From: ms(1, 0), To: ms(4, 0), Limit: 2},
		"admin@example.com",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 2 || first.NextCursor != "2" {
		t.Fatalf("first page = %+v", first)
	}
	if first.Records[0].At != ms(3, 10) || first.Records[1].At != ms(3, 9) {
		t.Fatalf("expected newest first, got %d then %d", first.Records[0].At, first.Records[1].At)
	}

	second, err := service.Records(
		context.Background(),
		RecordQuery{From: ms(1, 0), To: ms(4, 0), Limit: 2, Cursor: first.NextCursor},
		"admin@example.com",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 2 || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
}

func TestRecordsFilterByChat(t *testing.T) {
	service := New(testLedger(), testProjects(), nil)
	page, err := service.Records(
		context.Background(),
		RecordQuery{From: ms(1, 0), To: ms(4, 0), ChatID: "c1"},
		"admin@example.com",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 2 {
		t.Fatalf("records = %d, want 2", len(page.Records))
	}
	for _, record := range page.Records {
		if record.ChatID != "c1" {
			t.Fatalf("unexpected chat in page: %+v", record)
		}
	}
}

func TestRecordRunUsesProviderCostWhenReported(t *testing.T) {
	repo := newFakeRepository()
	service := New(repo, testProjects(), nil)
	service.RecordRun(context.Background(), RunEvent{
		At:        ms(9, 12),
		ChatID:    "c9",
		ProjectID: "aaaa1111",
		RunID:     "run-9",
		UserEmail: "Member@Example.com",
		Provider:  "claude",
		Usage: json.RawMessage(
			`{"input_tokens":100,"output_tokens":50,"total_cost_usd":0.9,"model":"claude-opus-4-5","num_turns":3}`,
		),
		Scheduled: true,
	})

	if len(repo.records) != 1 {
		t.Fatalf("records = %d, want 1", len(repo.records))
	}
	record := repo.records[0]
	if record.CostUSD == nil || *record.CostUSD != 0.9 || record.Estimated {
		t.Fatalf("expected exact provider cost, got %+v", record)
	}
	if record.Model != "claude-opus-4-5" || record.Turns != 3 || !record.Scheduled {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.ProjectSlug != "alpha" {
		t.Fatalf("slug = %q, want alpha", record.ProjectSlug)
	}
	if record.UserEmail != "member@example.com" {
		t.Fatalf("email = %q, want lower-cased", record.UserEmail)
	}
}

func TestRecordRunEstimatesAndMarksUnknownCost(t *testing.T) {
	tests := []struct {
		name      string
		usage     string
		model     string
		wantCost  *float64
		estimated bool
	}{
		{
			name:      "known model is estimated",
			usage:     `{"input_tokens":1000000,"output_tokens":0,"model":"gpt-5-codex"}`,
			wantCost:  cost(1.25),
			estimated: true,
		},
		{
			name:  "unknown model stays unpriced",
			usage: `{"input_tokens":1000,"model":"some-private-llm"}`,
		},
		{
			name:  "no tokens stays unpriced rather than zero",
			usage: `{"model":"kimi-k2"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepository()
			service := New(repo, testProjects(), nil)
			service.RecordRun(context.Background(), RunEvent{
				At:       ms(9, 12),
				ChatID:   "c9",
				Provider: "codex",
				Usage:    json.RawMessage(test.usage),
			})
			record := repo.records[0]
			if test.wantCost == nil {
				if record.CostUSD != nil {
					t.Fatalf("cost = %v, want unknown", *record.CostUSD)
				}
				return
			}
			if record.CostUSD == nil {
				t.Fatal("cost = unknown, want an estimate")
			}
			if math.Abs(*record.CostUSD-*test.wantCost) > 1e-9 {
				t.Fatalf("cost = %v, want %v", *record.CostUSD, *test.wantCost)
			}
			if record.Estimated != test.estimated {
				t.Fatalf("estimated = %t, want %t", record.Estimated, test.estimated)
			}
		})
	}
}

func TestRecordRunSkipsRunsWithoutAChat(t *testing.T) {
	repo := newFakeRepository()
	New(repo, testProjects(), nil).RecordRun(context.Background(), RunEvent{Provider: "claude"})
	if len(repo.records) != 0 {
		t.Fatalf("records = %d, want 0", len(repo.records))
	}
}

func TestRebuildIsIdempotentAndPreservesAttribution(t *testing.T) {
	chats := fakeChats{
		metas: []servicechat.Meta{
			{ID: "aaaa", ProjectID: "aaaa1111", Provider: servicechat.ProviderClaude, Model: "claude-sonnet-4-5"},
			{ID: "bbbb", Provider: servicechat.ProviderCodex, Model: "gpt-5-codex"},
		},
		events: map[servicechat.ID][]servicechat.Event{
			"aaaa": {
				{Seq: 1, T: ms(1, 9), Type: "user", Text: "hi"},
				{Seq: 2, T: ms(1, 10), Type: "complete", Usage: json.RawMessage(
					`{"input_tokens":100,"output_tokens":50,"total_cost_usd":0.42}`,
				)},
				{Seq: 3, T: ms(1, 11), Type: "error", Message: "boom"},
			},
			"bbbb": {
				{Seq: 1, T: ms(2, 9), Type: "complete", Usage: json.RawMessage(
					`{"input_tokens":1000000,"cache_read_input_tokens":300000,"output_tokens":0}`,
				)},
			},
		},
	}
	// A live record already exists for the first completion; its user, run id
	// and scheduled flag must survive the rebuild.
	repo := newFakeRepository(Record{
		At: ms(1, 10), ChatID: "aaaa", ProjectID: "aaaa1111", RunID: "live-run",
		UserEmail: "member@example.com", Provider: "claude", Scheduled: true,
	})
	service := New(repo, testProjects(), chats)

	result, err := service.Rebuild(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Chats != 2 || result.Records != 2 {
		t.Fatalf("result = %+v", result)
	}
	if result.PreservedActors != 1 {
		t.Fatalf("preserved actors = %d, want 1", result.PreservedActors)
	}

	first := repo.written[0]
	if first[0].ChatID != "aaaa" || first[0].UserEmail != "member@example.com" ||
		first[0].RunID != "live-run" || !first[0].Scheduled {
		t.Fatalf("attribution lost: %+v", first[0])
	}
	if first[0].CostUSD == nil || *first[0].CostUSD != 0.42 || first[0].Estimated {
		t.Fatalf("expected exact reported cost, got %+v", first[0])
	}
	if first[0].ProjectSlug != "alpha" {
		t.Fatalf("slug = %q, want alpha", first[0].ProjectSlug)
	}
	// The codex chat has no cost of its own, so the ledger estimates it and
	// derives provider/model from the chat metadata.
	if first[1].Provider != "codex" || first[1].Model != "gpt-5-codex" {
		t.Fatalf("unexpected provider/model: %+v", first[1])
	}
	if first[1].InputTokens != 700000 || first[1].CacheReadTokens != 300000 ||
		first[1].TotalTokens() != 1000000 {
		t.Fatalf("legacy Codex input was not normalized: %+v", first[1])
	}
	if first[1].CostUSD == nil || !first[1].Estimated ||
		math.Abs(*first[1].CostUSD-0.9125) > 1e-9 {
		t.Fatalf("expected a $0.9125 estimate, got %+v", first[1])
	}
	if first[1].RunID != "bbbb-1" {
		t.Fatalf("rebuilt run id = %q, want chat-seq derived", first[1].RunID)
	}

	if _, err := service.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := repo.written[1]
	if len(second) != len(first) {
		t.Fatalf("second rebuild wrote %d records, want %d", len(second), len(first))
	}
	for i := range first {
		if first[i] != second[i] && !recordsEqual(first[i], second[i]) {
			t.Fatalf("rebuild is not idempotent at %d:\n%+v\n%+v", i, first[i], second[i])
		}
	}
}

func TestRecordFromChatEventNormalizesOnlyLegacyCodexInput(t *testing.T) {
	prices, err := PriceTable{Models: []ModelPrice{{
		Match: "test-model", InputPerMTok: 3, OutputPerMTok: 15,
		CacheReadPerMTok: 0.3, CacheWritePerMTok: 3.75,
	}}}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	chat := servicechat.Meta{ID: "codex-chat", Provider: servicechat.ProviderCodex, Model: "test-model"}
	tests := []struct {
		name  string
		usage string
	}{
		{
			name:  "legacy inclusive input",
			usage: `{"input_tokens":10,"output_tokens":4,"cache_read_input_tokens":3,"cache_creation_input_tokens":2}`,
		},
		{
			name:  "versioned disjoint input",
			usage: `{"schema_version":1,"input_tokens":5,"output_tokens":4,"cache_read_input_tokens":3,"cache_creation_input_tokens":2}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record, ok := recordFromChatEvent(chat, servicechat.Event{
				Seq: 1, T: ms(1, 9), Type: "complete", Provider: servicechat.ProviderCodex,
				Usage: json.RawMessage(test.usage),
			}, nil, prices)
			if !ok {
				t.Fatal("completion was not rebuilt")
			}
			if record.InputTokens != 5 || record.CacheReadTokens != 3 ||
				record.CacheWriteTokens != 2 || record.OutputTokens != 4 ||
				record.TotalTokens() != 14 {
				t.Fatalf("record = %+v", record)
			}
			const wantCost = 0.0000834
			if record.CostUSD == nil || !record.Estimated ||
				math.Abs(*record.CostUSD-wantCost) > 1e-12 {
				t.Fatalf("cost = %v, want %v", record.CostUSD, wantCost)
			}
		})
	}
}

func TestRebuildPrefersLiveProviderAndModelForLegacyCompletion(t *testing.T) {
	at := ms(2, 9)
	chats := fakeChats{
		metas: []servicechat.Meta{{
			ID: "switched", Provider: servicechat.ProviderClaude, Model: "claude-sonnet-4-5",
		}},
		events: map[servicechat.ID][]servicechat.Event{
			"switched": {{
				Seq: 1, T: at, Type: "complete",
				Usage: json.RawMessage(`{"input_tokens":10,"cache_read_input_tokens":3}`),
			}},
		},
	}
	repo := newFakeRepository(Record{
		At: at, ChatID: "switched", Provider: "codex", Model: "gpt-5-codex",
	})
	service := New(repo, testProjects(), chats)
	if _, err := service.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	record := repo.written[0][0]
	if record.Provider != "codex" || record.Model != "gpt-5-codex" ||
		record.InputTokens != 7 || record.CacheReadTokens != 3 {
		t.Fatalf("live attribution was not preserved: %+v", record)
	}
}

// recordsEqual compares two records including the pointer-valued cost.
func recordsEqual(left, right Record) bool {
	leftCost, rightCost := left.CostUSD, right.CostUSD
	if (leftCost == nil) != (rightCost == nil) {
		return false
	}
	if leftCost != nil && *leftCost != *rightCost {
		return false
	}
	left.CostUSD, right.CostUSD = nil, nil
	return left == right
}

func TestSetPricesNormalizesAndStamps(t *testing.T) {
	repo := newFakeRepository()
	service := New(repo, testProjects(), nil)
	saved, err := service.SetPrices(context.Background(), PriceTable{
		Models: []ModelPrice{{Match: "MY-MODEL", InputPerMTok: 1, OutputPerMTok: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Models[0].Match != "my-model" || saved.UpdatedAt == 0 {
		t.Fatalf("unexpected saved table: %+v", saved)
	}
	if _, err := service.SetPrices(context.Background(), PriceTable{}); err != ErrInvalidPrice {
		t.Fatalf("err = %v, want ErrInvalidPrice", err)
	}
}

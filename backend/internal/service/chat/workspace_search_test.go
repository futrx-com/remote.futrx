package chat

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/project"
)

func TestWorkspaceSearchRanksChatTitleAndProjectMatches(t *testing.T) {
	projects := []project.Meta{{ID: "aaaa", Name: "test", Slug: "test"}}
	metas := []Meta{
		{ID: "aaaa", Title: "Postgres migration for user settings"},
		{ID: "bbbb", Title: "Set up Nginx reverse proxy with TLS", ProjectID: "aaaa"},
		{ID: "cccc", Title: "Test"},
	}
	result := SearchWorkspace(metas, projects, WorkspaceSearchQuery{Query: "test"}, time.UnixMilli(0))
	if len(result.Hits) != 2 {
		t.Fatalf("hits: %+v", result.Hits)
	}
	first, second := result.Hits[0], result.Hits[1]
	if first.Chat.ID != "cccc" || first.MatchedField != "title" || !slices.Equal(first.TitleSpans, []TextSpan{{0, 4}}) {
		t.Fatalf("title match: %+v", first)
	}
	if second.Chat.ID != "bbbb" || second.MatchedField != "project" || len(second.TitleSpans) != 0 {
		t.Fatalf("project match: %+v", second)
	}
}

func TestWorkspaceSearchFacetCountsAndDateSort(t *testing.T) {
	projects := []project.Meta{{ID: "aaaa", Name: "Ops"}}
	metas := []Meta{
		{ID: "aaaa", Title: "Zebra ops", ProjectID: "aaaa", Provider: ProviderClaude, Model: "opus", CreatedAt: 10, LastMessageAt: 100},
		{ID: "bbbb", Title: "Able ops", ProjectID: "aaaa", Provider: ProviderClaude, Model: "sonnet", CreatedAt: 20, LastMessageAt: 90},
		{ID: "cccc", Title: "Delta ops", ProjectID: "aaaa", Provider: ProviderCodex, Model: "gpt", CreatedAt: 30, LastMessageAt: 80},
		{ID: "dddd", Title: "Old ops", ProjectID: "aaaa", Provider: ProviderClaude, Model: "old", CreatedAt: 9, LastMessageAt: 200},
		{ID: "eeee", Title: "Other name", Provider: ProviderClaude, Model: "other", CreatedAt: 20},
	}
	from, to := int64(10), int64(30)
	selection := WorkspaceSearchSelection{
		Facets:    map[string][]string{"provider": {"claude"}, "model": {"opus", "sonnet"}},
		DateField: "createdAt", From: &from, To: &to,
	}
	result := SearchWorkspace(metas, projects, WorkspaceSearchQuery{
		WorkspaceSearchSelection: selection, Query: "ops", Sort: "title", WithCounts: true,
	}, time.UnixMilli(250))
	if !result.Counted || result.Total != 5 || len(result.Hits) != 2 ||
		result.Hits[0].Chat.ID != "bbbb" || result.Hits[1].Chat.ID != "aaaa" {
		t.Fatalf("unexpected sorted results: %+v", result)
	}
	if !reflect.DeepEqual(result.Counts["model"], map[string]int{"opus": 1, "sonnet": 1}) {
		t.Fatalf("model counts: %v", result.Counts["model"])
	}
	delete(selection.Facets, "model")
	if ids := WorkspaceMessageChatIDs(metas, projects, selection); !slices.Equal(ids, []ID{"aaaa", "bbbb", "eeee"}) {
		t.Fatalf("message chat scope: %v", ids)
	}
}

func TestWorkspaceMatcherPreservesFuzzyUnicodeAndUTF16Highlights(t *testing.T) {
	for _, tc := range []struct {
		query, field string
		spans        []TextSpan
	}{
		{"cafe", "😀 Café", []TextSpan{{3, 7}}},
		{"wss", "workspace sidebar state", []TextSpan{{0, 1}, {10, 11}, {18, 19}}},
		{"serch", "search settings", []TextSpan{{0, 6}}},
		{"→", "a → b", []TextSpan{{2, 3}}},
		{"ملفات۵", "ملفات 5", []TextSpan{{0, 5}, {6, 7}}},
		{"sidebar other", "workspace sidebar state", nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			hit, matched := scoreWorkspaceChat(Meta{Title: tc.field}, nil, nil, workspaceSearchTokens(tc.query))
			if matched != (tc.spans != nil) || (matched && !slices.Equal(hit.TitleSpans, tc.spans)) {
				t.Fatalf("match = %v spans = %v, want %v", matched, hit.TitleSpans, tc.spans)
			}
		})
	}
}

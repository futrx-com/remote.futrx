package chat

import (
	"cmp"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/futrx-com/remote.futrx.com/internal/service/project"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

type WorkspaceSearchSelection struct {
	Facets    map[string][]string `json:"facets"`
	DateField string              `json:"dateField"`
	From      *int64              `json:"from"`
	To        *int64              `json:"to"`
}

type WorkspaceSearchQuery struct {
	WorkspaceSearchSelection
	Query      string `json:"query"`
	Sort       string `json:"sort"`
	WithCounts bool   `json:"withCounts"`
}

type WorkspaceSearchHit struct {
	Chat         Meta          `json:"chat"`
	Project      *project.Meta `json:"project"`
	Score        float64       `json:"score"`
	TitleSpans   []TextSpan    `json:"titleSpans"`
	MatchedField string        `json:"matchedField"`
}

type WorkspaceSearchResult struct {
	Hits        []WorkspaceSearchHit      `json:"hits"`
	Counts      map[string]map[string]int `json:"counts"`
	FacetValues map[string][]string       `json:"facetValues"`
	Counted     bool                      `json:"counted"`
	Total       int                       `json:"total"`
}

func (q WorkspaceSearchQuery) Validate() error {
	if utf8.RuneCountInString(q.Query) > 512 {
		return ErrInvalidSearch
	}
	switch q.Sort {
	case "", "relevance", "recent", "oldest", "title":
		return q.WorkspaceSearchSelection.Validate()
	default:
		return ErrInvalidWorkspaceSearch
	}
}

// SearchWorkspace matches and sorts the caller's visible chats.
func SearchWorkspace(metas []Meta, projects []project.Meta, q WorkspaceSearchQuery, now time.Time) WorkspaceSearchResult {
	result := WorkspaceSearchResult{
		Hits: []WorkspaceSearchHit{}, Counts: newWorkspaceFacetCounts(),
		Counted: q.WithCounts, Total: len(metas),
	}
	index := workspaceProjectsByID(projects)
	tokens := workspaceSearchTokens(q.Query)
	for _, meta := range metas {
		if !q.includesDate(meta) {
			continue
		}
		p := index[meta.ProjectID]
		skills := workspaceSkillNames(meta)
		hit, matched := scoreWorkspaceChat(meta, p, skills, tokens)
		if !matched {
			continue
		}
		values := workspaceFacetValues(meta, p, skills)
		mask := values.matchMask(q.WorkspaceSearchSelection)
		if q.WithCounts {
			values.countMatches(result.Counts, mask)
		}
		if mask == allWorkspaceFacets {
			const window = int64((30 * 24 * time.Hour) / time.Millisecond)
			age := max(0, now.UnixMilli()-meta.LastMessageAt)
			hit.Score += 18 * (1 - float64(min(age, window))/float64(window))
			result.Hits = append(result.Hits, hit)
		}
	}

	result.FacetValues = workspaceFacetOptions(q.Facets, result.Counts)

	titles := collate.New(language.Und)
	slices.SortStableFunc(result.Hits, func(a, b WorkspaceSearchHit) int {
		switch q.Sort {
		case "oldest":
			return cmp.Compare(a.Chat.LastMessageAt, b.Chat.LastMessageAt)
		case "title":
			return titles.CompareString(a.Chat.Title, b.Chat.Title)
		case "", "relevance":
			if len(tokens) > 0 && a.Score != b.Score {
				return cmp.Compare(b.Score, a.Score)
			}
		}
		return cmp.Compare(b.Chat.LastMessageAt, a.Chat.LastMessageAt)
	})
	return result
}

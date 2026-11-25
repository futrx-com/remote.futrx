package chat

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SearchHit identifies a matching item in the active conversation branch.
type SearchHit struct {
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	StartSeq int64  `json:"startSeq"`
	EndSeq   int64  `json:"endSeq"`
	// Kind is the matching event's type, such as "user", "assistant_text",
	// "thinking" or "tool_use_end", so a client can show who said it.
	Kind string `json:"kind"`
	// T is when the matching event was recorded, in Unix milliseconds.
	T int64 `json:"t"`
	// Text is a plain-text excerpt; Markdown syntax is removed for display.
	Text string `json:"text"`
	// Highlights mark the matched words in Text as UTF-16 offsets, so a
	// browser can slice Text with them directly.
	Highlights []TextSpan `json:"highlights"`
}

// TextSpan is a half-open [Start, End) range of UTF-16 code units.
type TextSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// ChatSearchHit is a match found by searching across chats.
type ChatSearchHit struct {
	ChatID ID `json:"chatId"`
	SearchHit
}

// ChatSearchCursor continues a cross-chat search after its last hit. Hits are
// ordered by time, then chat, then sequence, all descending.
type ChatSearchCursor struct {
	T        int64
	ChatID   ID
	StartSeq int64
}

// ParseChatSearchCursor reads "<t>:<chatId>:<startSeq>"; empty is the start.
func ParseChatSearchCursor(raw string) (ChatSearchCursor, error) {
	if raw == "" {
		return ChatSearchCursor{}, nil
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return ChatSearchCursor{}, ErrInvalidSearchCursor
	}
	t, errT := strconv.ParseInt(parts[0], 10, 64)
	seq, errSeq := strconv.ParseInt(parts[2], 10, 64)
	if errT != nil || errSeq != nil || t <= 0 || !ValidID(ID(parts[1])) {
		return ChatSearchCursor{}, ErrInvalidSearchCursor
	}
	return ChatSearchCursor{T: t, ChatID: ID(parts[1]), StartSeq: seq}, nil
}

// compare orders positions newest first, using chat and sequence to settle ties.
func (c ChatSearchCursor) compare(other ChatSearchCursor) int {
	if order := cmp.Compare(other.T, c.T); order != 0 {
		return order
	}
	if order := cmp.Compare(other.ChatID, c.ChatID); order != 0 {
		return order
	}
	return cmp.Compare(other.StartSeq, c.StartSeq)
}

func (h ChatSearchHit) position() ChatSearchCursor {
	return ChatSearchCursor{T: h.T, ChatID: h.ChatID, StartSeq: h.StartSeq}
}

var ErrInvalidSearchCursor = errors.New("invalid search cursor")

type TranscriptSearchSource interface {
	// SearchTranscriptPage returns matches newest first; a positive before
	// continues after a previous page's oldest StartSeq.
	SearchTranscriptPage(ctx context.Context, id ID, query string, limit int, before int64) ([]SearchHit, error)
	// SearchTranscriptUntil returns matches newest first recorded at or
	// before maxT, in Unix milliseconds, when maxT is positive.
	SearchTranscriptUntil(ctx context.Context, id ID, query string, limit int, maxT int64) ([]SearchHit, error)
}

var ErrInvalidSearch = errors.New("search query must contain at most 512 characters")

func WithTranscriptSearchSource(source TranscriptSearchSource) Option {
	return func(s *Service) { s.transcriptSearch = source }
}

func (s *Service) SearchTranscript(ctx context.Context, id ID, query string, limit int, before int64) ([]SearchHit, error) {
	if !ValidID(id) {
		return nil, ErrInvalidID
	}
	query, limit, err := normalizeSearchRequest(query, limit)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return []SearchHit{}, nil
	}
	if s.transcriptSearch == nil {
		return nil, ErrTranscriptUnavailable
	}
	return s.transcriptSearch.SearchTranscriptPage(ctx, id, query, limit, max(0, before))
}

// SearchChats finds messages across the given chats, newest first. Callers
// pass only chats the requester may read. A chat without a searchable
// transcript is skipped rather than failing the whole search.
func (s *Service) SearchChats(ctx context.Context, ids []ID, query string, limit int, cursor ChatSearchCursor) ([]ChatSearchHit, error) {
	query, limit, err := normalizeSearchRequest(query, limit)
	if err != nil {
		return nil, err
	}
	if query == "" || len(ids) == 0 {
		return []ChatSearchHit{}, nil
	}
	if s.transcriptSearch == nil {
		return nil, ErrTranscriptUnavailable
	}
	out := []ChatSearchHit{}
	for _, id := range ids {
		if !ValidID(id) {
			continue
		}
		// Hits at the cursor's millisecond may still follow it, so read up to
		// and including that time and let the cursor settle ties.
		hits, err := s.transcriptSearch.SearchTranscriptUntil(ctx, id, query, limit, cursor.T)
		if errors.Is(err, ErrTranscriptUnavailable) || errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, hit := range hits {
			match := ChatSearchHit{ChatID: id, SearchHit: hit}
			if cursor.T == 0 || match.position().compare(cursor) > 0 {
				out = append(out, match)
			}
		}
	}
	slices.SortFunc(out, func(a, b ChatSearchHit) int {
		return a.position().compare(b.position())
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func normalizeSearchRequest(query string, limit int) (string, int, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > 512 {
		return "", 0, ErrInvalidSearch
	}
	if limit <= 0 {
		limit = 50
	}
	return query, min(limit, 100), nil
}

// BranchForker creates an independent branch referencing a retained prefix.
// A cutoff of -1 captures the current tip atomically; zero creates an empty branch.
type BranchForker interface {
	ForkChat(context.Context, ID, Meta, int64) (Meta, error)
}

func WithBranchForker(forker BranchForker) Option {
	return func(s *Service) { s.branchForker = forker }
}

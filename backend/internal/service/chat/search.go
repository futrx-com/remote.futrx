package chat

import (
	"context"
	"errors"
	"strings"
)

// SearchHit identifies a matching item in the active conversation branch.
type SearchHit struct {
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	StartSeq int64  `json:"startSeq"`
	EndSeq   int64  `json:"endSeq"`
	Text     string `json:"text"`
}

type TranscriptSearchSource interface {
	SearchTranscript(context.Context, ID, string, int) ([]SearchHit, error)
}

var ErrInvalidSearch = errors.New("search query must contain at most 512 characters")

func WithTranscriptSearchSource(source TranscriptSearchSource) Option {
	return func(s *Service) { s.transcriptSearch = source }
}

func (s *Service) SearchTranscript(ctx context.Context, id ID, query string, limit int) ([]SearchHit, error) {
	if !ValidID(id) {
		return nil, ErrInvalidID
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 512 {
		return nil, ErrInvalidSearch
	}
	if query == "" {
		return []SearchHit{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if s.transcriptSearch == nil {
		return nil, ErrTranscriptProjectionUnavailable
	}
	return s.transcriptSearch.SearchTranscript(ctx, id, query, limit)
}

// BranchForker creates an independent branch referencing a retained prefix.
// A cutoff of -1 captures the current tip atomically; zero creates an empty branch.
type BranchForker interface {
	ForkChat(context.Context, ID, Meta, int64) (Meta, error)
}

func WithBranchForker(forker BranchForker) Option {
	return func(s *Service) { s.branchForker = forker }
}

package chat

import "context"

// BranchRewinder moves the active branch without loading its retained events.
type BranchRewinder interface {
	RewindBranch(context.Context, ID, int64) error
}

type HistoryVersionSource interface {
	HistoryVersion(context.Context, ID) (string, error)
}

func WithBranchRewinder(rewinder BranchRewinder) Option {
	return func(s *Service) { s.branchRewinder = rewinder }
}

// IdleChatMutator serializes branch changes against starting a run.
type IdleChatMutator interface {
	MutateIdle(ID, func() error) error
}

package chat

import "errors"

var (
	ErrInvalidID              = errors.New("invalid chat id")
	ErrInvalidProvider        = errors.New("invalid chat provider")
	ErrInvalidTmuxSession     = errors.New("invalid tmux session")
	ErrInvalidRewindTimestamp = errors.New("invalid rewind timestamp")
	ErrChatRunning            = errors.New("chat has an active run")
	ErrNotFound               = errors.New("chat not found")
	// ErrProjectNotFound is what a ProjectResolver reports for a project that
	// no longer exists. A chat outlives its project, so this is a state the
	// chat service has to answer rather than an error to propagate.
	ErrProjectNotFound                 = errors.New("project not found")
	ErrTranscriptContentNotFound       = errors.New("transcript content not found")
	ErrTranscriptProjectionUnavailable = errors.New("transcript projection unavailable")
)

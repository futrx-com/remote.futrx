package chat

import "errors"

var (
	ErrInvalidID              = errors.New("invalid chat id")
	ErrInvalidProvider        = errors.New("invalid chat provider")
	ErrInvalidTmuxSession     = errors.New("invalid tmux session")
	ErrInvalidRewindTimestamp = errors.New("invalid rewind timestamp")
	ErrChatRunning            = errors.New("chat has an active run")
	ErrNotFound               = errors.New("chat not found")
)

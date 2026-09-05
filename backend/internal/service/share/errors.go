package share

import "errors"

var (
	ErrInvalidPort      = errors.New("preview port must be between 1024 and 65535")
	ErrPortNotShareable = errors.New("this port is platform plumbing (agent browser, IDE, or DevTools) and cannot be shared publicly")
	ErrInvalidTTL       = errors.New("share lifetime must be between 1 hour and 30 days")
	ErrTooManyShares    = errors.New("this project already has the maximum number of active share links")
	ErrNotFound         = errors.New("share link not found")
	ErrUnavailable      = errors.New("share link store is not configured")
)

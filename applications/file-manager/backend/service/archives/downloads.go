// Package archives owns admission and the time budget for folder downloads.
// It does not know how the archive is built or how its bytes are delivered.
package archives

import (
	"context"
	"errors"
	"time"
)

var ErrBusy = errors.New("other downloads are still being prepared")

// Downloads bounds concurrent archive builds for one installation. Waiting
// for a slot and building the ZIP share the same timeout.
type Downloads struct {
	slots   chan struct{}
	timeout time.Duration
}

func NewDownloads(concurrency int, timeout time.Duration) *Downloads {
	return &Downloads{slots: make(chan struct{}, concurrency), timeout: timeout}
}

func (d *Downloads) Build(build func(context.Context) ([]byte, error)) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	select {
	case d.slots <- struct{}{}:
		defer func() { <-d.slots }()
	case <-ctx.Done():
		return nil, ErrBusy
	}
	return build(ctx)
}

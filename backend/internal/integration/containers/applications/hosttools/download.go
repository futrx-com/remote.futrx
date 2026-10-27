package hosttools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

const downloadIdleTimeout = 30 * time.Second

// The deadline covers headers and resets when body bytes arrive. Unlike a
// total transfer deadline, it allows a large download that is making progress.
func fetchHTTPS(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	return fetchDownload(ctx, http.DefaultClient, rawURL, downloadIdleTimeout)
}

func fetchDownload(parent context.Context, client *http.Client, rawURL string, idle time.Duration) (io.ReadCloser, error) {
	ctx, cancel := context.WithCancelCause(parent)
	stalled := fmt.Errorf("download connection stalled for %s; retry the installation to try again", idle)
	timer := time.AfterFunc(idle, func() { cancel(stalled) })
	closeAttempt := func() { timer.Stop(); cancel(nil) }
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		closeAttempt()
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(context.Cause(ctx), stalled) {
			err = stalled
		}
		closeAttempt()
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		closeAttempt()
		return nil, fmt.Errorf("download %s: HTTP %d", rawURL, response.StatusCode)
	}
	timer.Reset(idle)
	return &downloadBody{ReadCloser: response.Body, ctx: ctx, timer: timer, idle: idle, closeAttempt: closeAttempt,
		total: response.ContentLength, output: svc.InstallOutput(parent)}, nil
}

type downloadBody struct {
	io.ReadCloser
	ctx             context.Context
	timer           *time.Timer
	idle            time.Duration
	closeAttempt    func()
	total, received int64
	lastReport      time.Time
	output          io.Writer
}

func (b *downloadBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	if n > 0 {
		b.timer.Reset(b.idle)
		b.received += int64(n)
		if time.Since(b.lastReport) >= time.Second || err == io.EOF || b.received == b.total {
			if b.total > 0 {
				fmt.Fprintf(b.output, "Downloaded %.1f / %.1f MB\n", float64(b.received)/1e6, float64(b.total)/1e6)
			} else {
				fmt.Fprintf(b.output, "Downloaded %.1f MB\n", float64(b.received)/1e6)
			}
			b.lastReport = time.Now()
		}
	}
	if err == io.EOF {
		b.timer.Stop()
	}
	if err != nil && context.Cause(b.ctx) != nil {
		err = context.Cause(b.ctx)
	}
	return n, err
}

func (b *downloadBody) Close() error {
	b.closeAttempt()
	return b.ReadCloser.Close()
}

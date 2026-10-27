package command

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

type outputKey struct{}
type captureLimitKey struct{}

// WithCaptureLimit retains only the last limit bytes while the process runs.
// Other callers keep the default complete capture.
func WithCaptureLimit(ctx context.Context, limit int) context.Context {
	return context.WithValue(ctx, captureLimitKey{}, limit)
}

// WithOutput tees selected command output to a caller-owned log. Commands
// without this option retain their normal capture-only behavior.
func WithOutput(ctx context.Context, output io.Writer) context.Context {
	return context.WithValue(ctx, outputKey{}, output)
}

func CaptureOutput(ctx context.Context, cmd *exec.Cmd) (string, error) {
	var buffer interface {
		io.Writer
		String() string
	} = &bytes.Buffer{}
	if limit, _ := ctx.Value(captureLimitKey{}).(int); limit > 0 {
		buffer = &tailBuffer{data: make([]byte, 0, limit), limit: limit}
	}
	var output io.Writer = buffer
	if observer, ok := ctx.Value(outputKey{}).(io.Writer); ok {
		output = io.MultiWriter(buffer, observer)
	}
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	return buffer.String(), err
}

type tailBuffer struct {
	data  []byte
	limit int
}

func (b *tailBuffer) Write(data []byte) (int, error) {
	n := len(data)
	if n >= b.limit {
		b.data = append(b.data[:0], data[n-b.limit:]...)
		return n, nil
	}
	if excess := len(b.data) + n - b.limit; excess > 0 {
		copy(b.data, b.data[excess:])
		b.data = b.data[:len(b.data)-excess]
	}
	b.data = append(b.data, data...)
	return n, nil
}

func (b *tailBuffer) String() string { return string(b.data) }

// Package containerio runs File Manager's container program for the host
// backend. The command and its arguments are fixed; browser input is written
// to stdin as JSON and never becomes a program argument or shell text.
package containerio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"futrx.local/catalog/applications/file-manager/backend/config"
)

// Exit statuses of backend/container/internal/transport/cli.
const (
	exitInvalidPath = 3
	exitNotFound    = 4
	exitTooLarge    = 5
)

var (
	ErrInvalidPath = errors.New("invalid path")
	ErrNotFound    = errors.New("not found")
	ErrTooLarge    = errors.New("result exceeds the download size limit")
)

// Request is the JSON the container program reads on stdin.
type Request struct {
	Path  string `json:"path,omitempty"`
	Query string `json:"query,omitempty"`
}

// Runner runs one container command and returns its stdout.
type Runner func(ctx context.Context, container, command string, request Request, limit int64) ([]byte, error)

// Run executes the File Manager program in container through `lxc exec`.
func Run(ctx context.Context, container, command string, request Request, limit int64) ([]byte, error) {
	input, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	stdout := &limitedBuffer{limit: limit}
	stderr := &truncatingBuffer{limit: 4096}
	process := exec.CommandContext(ctx, "lxc", "exec", container, "--", config.ContainerCommand, command)
	process.Stdin = bytes.NewReader(input)
	process.Stdout = stdout
	process.Stderr = stderr
	process.WaitDelay = time.Second
	err = process.Run()
	if stdout.exceeded {
		return nil, ErrTooLarge
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, commandError(err, stderr.buffer.String())
	}
	return stdout.buffer.Bytes(), nil
}

func commandError(err error, stderr string) error {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		switch exit.ExitCode() {
		case exitInvalidPath:
			return ErrInvalidPath
		case exitNotFound:
			return ErrNotFound
		case exitTooLarge:
			return ErrTooLarge
		}
	}
	return fmt.Errorf("file-manager in project container: %w: %s", err, strings.TrimSpace(stderr))
}

// limitedBuffer fails the write that would exceed limit, which stops the
// copy from the container instead of buffering an unbounded result.
type limitedBuffer struct {
	limit    int64
	buffer   bytes.Buffer
	exceeded bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if int64(b.buffer.Len()+len(data)) > b.limit {
		b.exceeded = true
		return 0, ErrTooLarge
	}
	return b.buffer.Write(data)
}

// truncatingBuffer keeps the start of a diagnostic stream and accepts the rest
// unread, so a chatty failure cannot turn into a write error.
type truncatingBuffer struct {
	limit  int
	buffer bytes.Buffer
}

func (b *truncatingBuffer) Write(data []byte) (int, error) {
	if room := b.limit - b.buffer.Len(); room > 0 {
		b.buffer.Write(data[:min(room, len(data))])
	}
	return len(data), nil
}

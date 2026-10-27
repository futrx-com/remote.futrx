package runtime

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// converseStderrLimit bounds the stderr a conversation keeps for its error.
const converseStderrLimit = 2048

// Converse runs cmd in a process group of its own and hands its stdin and
// stdout to talk, for a short request-and-answer exchange with a CLI.
//
// When talk returns, stdin is closed so the CLI can exit by itself, and after
// grace the whole group is killed: an npm-installed CLI is a wrapper whose
// child does the work, and neither may outlive the exchange. If ctx ends
// first, the group is killed at once and talk's pipes are closed, so talk
// returns even if a descendant that left the group still holds them.
//
// Converse returns talk's error, or ctx's when ctx ended first, and the start
// of what the CLI wrote to stderr.
func Converse(
	ctx context.Context,
	cmd *exec.Cmd,
	grace time.Duration,
	talk func(stdin io.Writer, stdout io.Reader) error,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Wait must not outlast grace because a descendant kept stderr open.
	cmd.WaitDelay = grace
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr := &limitedBuffer{limit: converseStderrLimit}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	group := -cmd.Process.Pid
	killGroup := func() { _ = syscall.Kill(group, syscall.SIGKILL) }

	talked := make(chan error, 1)
	go func() { talked <- talk(stdin, stdout) }()
	var talkErr error
	select {
	case talkErr = <-talked:
	case <-ctx.Done():
		killGroup()
		_ = stdin.Close()
		_ = stdout.Close()
		<-talked
		talkErr = ctx.Err()
	}

	_ = stdin.Close()
	exited := make(chan struct{})
	go func() {
		// Nothing more is read; draining lets the CLI finish writing and exit.
		_, _ = io.Copy(io.Discard, stdout)
		_ = cmd.Wait()
		close(exited)
	}()
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-exited:
	case <-timer.C:
		killGroup()
		_ = stdout.Close()
		<-exited
	}
	// A descendant may still run after the leader exits; the group ID must
	// not be left to one that could later be reused.
	killGroup()
	return strings.TrimSpace(stderr.String()), talkErr
}

// limitedBuffer keeps the first limit bytes written to it.
type limitedBuffer struct {
	strings.Builder
	limit int
}

// Write always reports the whole of data as written, so the copy feeding it
// never stops early and the CLI never blocks on a full stderr pipe.
func (b *limitedBuffer) Write(data []byte) (int, error) {
	if room := b.limit - b.Len(); room > 0 {
		kept := data
		if len(kept) > room {
			kept = kept[:room]
		}
		b.Builder.Write(kept)
	}
	return len(data), nil
}

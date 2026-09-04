package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type ProcessOptions struct {
	Name               string
	LogID              string
	Provider           agent.ProviderID
	ConversationID     string
	StdoutMaxLineBytes int
	StderrMaxLineBytes int
}

type ProcessError struct {
	Err    error
	Stderr string
}

func (e *ProcessError) Error() string { return e.Err.Error() }
func (e *ProcessError) Unwrap() error { return e.Err }

func ErrorStderr(err error) string {
	var processErr *ProcessError
	if errors.As(err, &processErr) {
		return processErr.Stderr
	}
	return ""
}

func RunProcess(
	ctx context.Context,
	cmd *exec.Cmd,
	parser agent.LineParser,
	emit func(agent.Event),
	opts ProcessOptions,
) error {
	if emit == nil {
		emit = func(agent.Event) {}
	}
	if parser == nil {
		return errors.New("agent parser is required")
	}
	name := opts.Name
	if name == "" {
		name = "agent"
	}
	logID := opts.LogID
	if logID == "" {
		logID = opts.ConversationID
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn %s: %w", name, err)
	}

	stderrDone := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 0, 8192), maxBytes(opts.StderrMaxLineBytes, 1<<20))
		var captured strings.Builder
		for sc.Scan() {
			line := sc.Text()
			log.Printf("%s[%s] stderr: %s", name, logID, line)
			if captured.Len() < 64<<10 {
				captured.WriteString(line)
				captured.WriteByte('\n')
			}
		}
		stderrDone <- captured.String()
	}()

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), maxBytes(opts.StdoutMaxLineBytes, 16<<20))
	runFailed := false
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		events, err := parser.ParseLine(line)
		if err != nil {
			log.Printf("%s[%s] parse: %v line=%s", name, logID, err, truncate(string(line), 200))
			continue
		}
		for _, ev := range events {
			if ev.Type == agent.EventRunFailed {
				runFailed = true
			}
			emit(ev)
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		emit(agent.Event{
			T:              time.Now().UnixMilli(),
			Type:           agent.EventError,
			Provider:       opts.Provider,
			ConversationID: opts.ConversationID,
			Message:        "stdout: " + err.Error(),
		})
	}

	// Drain stderr before Wait: Wait closes the pipes returned by StderrPipe,
	// which can truncate an in-flight read in the goroutine above.
	stderrText := <-stderrDone
	err = cmd.Wait()
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	if err != nil && runFailed {
		return &ProcessError{Err: agent.ErrRunFailed, Stderr: stderrText}
	}
	if err != nil {
		return &ProcessError{Err: err, Stderr: stderrText}
	}
	return nil
}

func maxBytes(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

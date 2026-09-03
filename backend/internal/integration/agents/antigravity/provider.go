package antigravity

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

const signInHint = "antigravity is not signed in — open this chat's Terminal, run `agy`, and complete the sign-in URL + code flow, then retry"

type Provider struct {
	projectPreparer agent.ProjectPreparer
	binary          string
}

func newProvider(projectPreparer agent.ProjectPreparer, binary string) *Provider {
	return &Provider{projectPreparer: projectPreparer, binary: binary}
}

func (p *Provider) ID() agent.ProviderID {
	return agent.ProviderAntigravity
}

func (p *Provider) Parser(req agent.RunRequest) agent.LineParser {
	return NewParser(req)
}

func (p *Provider) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	if emit == nil {
		emit = func(agent.Event) {}
	}
	if req.Provider == "" {
		req.Provider = agent.ProviderAntigravity
	}
	// agy has no fork primitive; a forked chat simply starts fresh.
	if req.Fork {
		req.ResumeID = ""
	}

	cmd, containerName, err := p.buildCmd(ctx, req, p.args(req), emit)
	if err != nil {
		return err
	}

	store := conversationStore{containerName: containerName}
	var before map[string]struct{}
	if req.ResumeID == "" {
		before = store.list(ctx)
	}

	output, runErr := p.streamPrintRun(ctx, cmd, req, emit)
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	if runErr != nil {
		message := fmt.Sprintf("agy run failed: %v", runErr)
		if tail := strings.TrimSpace(output); tail != "" {
			message = fmt.Sprintf("%s; output: %s", message, tail)
		}
		if isSignInError(output) {
			message = signInHint
		}
		emit(agent.Event{
			T:              time.Now().UnixMilli(),
			Type:           agent.EventRunFailed,
			Provider:       agent.ProviderAntigravity,
			ConversationID: req.ConversationID,
			Message:        message,
		})
		return agent.ErrRunFailed
	}

	if req.ResumeID == "" {
		if id := store.newConversation(ctx, before); id != "" {
			emit(agent.Event{
				T:              time.Now().UnixMilli(),
				Type:           agent.EventSessionUpdated,
				Provider:       agent.ProviderAntigravity,
				ConversationID: req.ConversationID,
				SessionID:      id,
			})
		}
	}
	// agy print mode reports no tokens and no price, so the completion event
	// carries the model alone; cost is recorded as unknown downstream.
	emit(agent.Event{
		T:              time.Now().UnixMilli(),
		Type:           agent.EventRunCompleted,
		Provider:       agent.ProviderAntigravity,
		ConversationID: req.ConversationID,
		Usage:          agent.Usage{Model: req.Model}.Raw(),
	})
	return nil
}

// streamPrintRun executes one agy print-mode process, forwarding stdout to the
// chat as raw text deltas. Chunked reads (not line scanning) keep blank lines
// intact so markdown paragraphs survive, and text streams as it arrives. The
// combined output tail is returned for error reporting.
func (p *Provider) streamPrintRun(
	ctx context.Context,
	cmd *exec.Cmd,
	req agent.RunRequest,
	emit func(agent.Event),
) (string, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("spawn agy: %w", err)
	}

	var tail tailBuffer
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 8192), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("agy[%s] stderr: %s", req.ConversationID, line)
			tail.append(line + "\n")
		}
	}()

	reader := bufio.NewReader(stdout)
	chunk := make([]byte, 4096)
	for {
		n, readErr := reader.Read(chunk)
		if n > 0 {
			text := string(chunk[:n])
			tail.append(text)
			emit(agent.Event{
				T:              time.Now().UnixMilli(),
				Type:           agent.EventAssistantTextDelta,
				Provider:       agent.ProviderAntigravity,
				ConversationID: req.ConversationID,
				ItemKind:       agent.ItemMessage,
				Text:           text,
			})
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) && ctx.Err() == nil {
				log.Printf("agy[%s] stdout: %v", req.ConversationID, readErr)
			}
			break
		}
	}

	err = cmd.Wait()
	<-stderrDone
	return tail.String(), err
}

func isSignInError(output string) bool {
	lowered := strings.ToLower(output)
	return strings.Contains(lowered, "sign in") || strings.Contains(lowered, "signed out") ||
		strings.Contains(lowered, "not authenticated")
}

// tailBuffer keeps the last few KB of process output for error messages.
// Appended to from both the stdout loop and the stderr goroutine.
type tailBuffer struct {
	mu   sync.Mutex
	data []byte
}

const tailBufferLimit = 4096

func (b *tailBuffer) append(s string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, s...)
	if len(b.data) > tailBufferLimit {
		b.data = b.data[len(b.data)-tailBufferLimit:]
	}
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

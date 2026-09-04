package antigravity

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

	output, runErr := streamPrintRun(ctx, cmd, req, emit)
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

func isSignInError(output string) bool {
	lowered := strings.ToLower(output)
	return strings.Contains(lowered, "sign in") || strings.Contains(lowered, "signed out") ||
		strings.Contains(lowered, "not authenticated")
}

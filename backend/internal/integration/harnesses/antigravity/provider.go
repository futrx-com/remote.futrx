package antigravity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/runtime"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

const signInHint = "antigravity is not signed in — open this chat's Terminal application, run `agy`, and complete the sign-in URL + code flow, then retry"

type Provider struct {
	accountAuthorizer agentauth.AccountAuthorizer
	projectPreparer   harness.ProjectPreparer
	binary            string
}

func newProvider(projectPreparer harness.ProjectPreparer, binary string, accountAuthorizer agentauth.AccountAuthorizer) *Provider {
	return &Provider{projectPreparer: projectPreparer, binary: binary, accountAuthorizer: accountAuthorizer}
}

func (p *Provider) ID() harness.ProviderID {
	return harness.ProviderAntigravity
}

func (p *Provider) Parser(req harness.RunRequest) harness.LineParser {
	if req.TrackInputAcceptance {
		return newStreamParser(req)
	}
	return NewParser(req)
}

func (p *Provider) Run(ctx context.Context, req harness.RunRequest, emit func(harness.Event)) error {
	if err := agentauth.RequireAccountUse(ctx, p.accountAuthorizer, harness.ProviderAntigravity, ""); err != nil {
		return err
	}
	if strings.TrimSpace(req.AccountID) != "" {
		return agentauth.ErrAccountNotFound
	}
	if emit == nil {
		emit = func(harness.Event) {}
	}
	if req.Provider == "" {
		req.Provider = harness.ProviderAntigravity
	}
	// agy has no fork primitive; a forked chat simply starts fresh.
	if req.Fork {
		req.ResumeID = ""
	}

	cmd, containerName, err := p.buildCmd(ctx, req, p.args(req), emit)
	if err != nil {
		return err
	}

	if err := agentauth.RequireAccountUse(ctx, p.accountAuthorizer, harness.ProviderAntigravity, ""); err != nil {
		return err
	}
	if req.TrackInputAcceptance {
		return agentruntime.RunProcess(ctx, cmd, p.Parser(req), emit, agentruntime.ProcessOptions{
			Name: "agy", LogID: req.ConversationID, Provider: req.Provider, ConversationID: req.ConversationID,
		})
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
		emit(harness.Event{
			T:              time.Now().UnixMilli(),
			Type:           harness.EventRunFailed,
			Provider:       harness.ProviderAntigravity,
			ConversationID: req.ConversationID,
			Message:        message,
		})
		return harness.ErrRunFailed
	}

	if req.ResumeID == "" {
		if id := store.newConversation(ctx, before); id != "" {
			emit(harness.Event{
				T:              time.Now().UnixMilli(),
				Type:           harness.EventSessionUpdated,
				Provider:       harness.ProviderAntigravity,
				ConversationID: req.ConversationID,
				SessionID:      id,
			})
		}
	}
	// agy print mode reports no tokens and no price, so the completion event
	// carries the model alone; cost is recorded as unknown downstream.
	emit(harness.Event{
		T:              time.Now().UnixMilli(),
		Type:           harness.EventRunCompleted,
		Provider:       harness.ProviderAntigravity,
		ConversationID: req.ConversationID,
		Usage:          harness.Usage{Model: req.Model}.Raw(),
	})
	return nil
}

func isSignInError(output string) bool {
	lowered := strings.ToLower(output)
	return strings.Contains(lowered, "sign in") || strings.Contains(lowered, "signed out") ||
		strings.Contains(lowered, "not authenticated")
}

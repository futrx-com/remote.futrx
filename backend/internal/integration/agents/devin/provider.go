package devin

import (
	"context"
	"log"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/devinharness"
)

// Provider adapts the Devin CLI (`devin acp`) as a headless agent provider.
// The CLI speaks the Agent Client Protocol (ACP) over stdin/stdout, so runs
// surface structured tool calls, usage, and sessions. The devinharness package
// owns the JSON-RPC state machine; this provider owns command construction,
// credential syncing, and the agent.Provider contract.
type Provider struct {
	projectPreparer       agent.ProjectPreparer
	credentialCollector   provisioning.CredentialCollector
	profile               provisioning.Profile
	credentialSyncTimeout time.Duration
}

func newProvider(
	projectPreparer agent.ProjectPreparer,
	credentialCollector provisioning.CredentialCollector,
	profile provisioning.Profile,
	credentialSyncTimeout time.Duration,
) *Provider {
	return &Provider{
		projectPreparer:       projectPreparer,
		credentialCollector:   credentialCollector,
		profile:               profile.Clone(),
		credentialSyncTimeout: credentialSyncTimeout,
	}
}

func (p *Provider) ID() agent.ProviderID {
	return agent.ProviderDevin
}

// Parser returns the line parser for a run. The ACP harness emits agent.Event
// values directly from its JSON-RPC state machine, so there is no line-oriented
// output to parse. The returned parser is a no-op that satisfies the
// agent.LineParser contract for callers that still request one.
func (p *Provider) Parser(req agent.RunRequest) agent.LineParser {
	return NewParser(req)
}

// Run executes one Devin ACP turn. It builds the command, delegates to
// devinharness.Run for the JSON-RPC state machine, and syncs credentials back
// from the container after a successful project-scoped run.
func (p *Provider) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	if emit == nil {
		emit = func(agent.Event) {}
	}
	if req.Provider == "" {
		req.Provider = agent.ProviderDevin
	}
	// Devin has no fork primitive; a forked chat simply starts fresh.
	if req.Fork {
		req.ResumeID = ""
	}

	cmd, containerName, err := p.buildCmd(ctx, req, p.args(req), emit)
	if err != nil {
		return err
	}
	err = devinharness.Run(ctx, cmd, req, "Devin", emit)
	if err == nil && containerName != "" && p.credentialCollector != nil {
		syncCtx, cancel := context.WithTimeout(context.Background(), p.credentialSyncTimeout)
		defer cancel()
		if syncErr := p.credentialCollector.SyncFromContainer(syncCtx, containerName, p.profile.Credentials); syncErr != nil {
			log.Printf("devin[%s] sync auth from %s: %v", req.ConversationID, containerName, syncErr)
		}
	}
	return err
}

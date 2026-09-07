package kimi

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

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
	return agent.ProviderKimi
}

func (p *Provider) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	if emit == nil {
		emit = func(agent.Event) {}
	}
	if req.Mode != "" && req.Mode != agent.RunModeDefault && req.Mode != agent.RunModePlan {
		return fmt.Errorf("unsupported Kimi mode %q", req.Mode)
	}
	run := newServerRun(req, emit)
	cmd, containerName, err := p.buildCmd(ctx, req, bridgeArgs(), emit)
	if err != nil {
		return err
	}
	err = run.executeCommand(ctx, cmd)
	if containerName != "" && p.credentialCollector != nil {
		syncCtx, cancel := context.WithTimeout(context.Background(), p.credentialSyncTimeout)
		defer cancel()
		if syncErr := p.credentialCollector.SyncFromContainer(syncCtx, containerName, p.profile.Credentials); syncErr != nil {
			log.Printf("kimi[%s] sync auth from %s: %v", req.ConversationID, containerName, syncErr)
		}
	}
	return run.finish(ctx, err)
}

package kimi

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
	"github.com/futrx-com/remote.futrx.com/internal/harness/provisioning"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/runtime"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

type Provider struct {
	accountAuthorizer     agentauth.AccountAuthorizer
	projectPreparer       harness.ProjectPreparer
	credentialCollector   provisioning.CredentialCollector
	profile               provisioning.Profile
	credentialSyncTimeout time.Duration
}

func newProvider(
	projectPreparer harness.ProjectPreparer,
	credentialCollector provisioning.CredentialCollector,
	profile provisioning.Profile,
	credentialSyncTimeout time.Duration,
	accountAuthorizer agentauth.AccountAuthorizer,
) *Provider {
	return &Provider{
		projectPreparer:       projectPreparer,
		accountAuthorizer:     accountAuthorizer,
		credentialCollector:   credentialCollector,
		profile:               profile.Clone(),
		credentialSyncTimeout: credentialSyncTimeout,
	}
}

func (p *Provider) ID() harness.ProviderID {
	return harness.ProviderKimi
}

func (p *Provider) Parser(req harness.RunRequest) harness.LineParser {
	return NewParser(req)
}

func (p *Provider) Run(ctx context.Context, req harness.RunRequest, emit func(harness.Event)) error {
	if err := agentauth.RequireAccountUse(ctx, p.accountAuthorizer, harness.ProviderKimi, ""); err != nil {
		return err
	}
	if strings.TrimSpace(req.AccountID) != "" {
		return agentauth.ErrAccountNotFound
	}
	if emit == nil {
		emit = func(harness.Event) {}
	}
	if req.Provider == "" {
		req.Provider = harness.ProviderKimi
	}
	// kimi-code has no fork primitive; a forked chat simply starts fresh.
	if req.Fork {
		req.ResumeID = ""
	}

	cmd, containerName, err := p.buildCmd(ctx, req, p.args(req), emit)
	if err != nil {
		return err
	}
	if err := agentauth.RequireAccountUse(ctx, p.accountAuthorizer, harness.ProviderKimi, ""); err != nil {
		return err
	}
	err = agentruntime.RunProcess(ctx, cmd, p.Parser(req), emit, agentruntime.ProcessOptions{
		Name:           "kimi",
		LogID:          req.ConversationID,
		Provider:       harness.ProviderKimi,
		ConversationID: req.ConversationID,
	})
	if err == nil && containerName != "" && p.credentialCollector != nil {
		syncCtx, cancel := context.WithTimeout(context.Background(), p.credentialSyncTimeout)
		defer cancel()
		if syncErr := p.credentialCollector.SyncFromContainer(syncCtx, containerName, p.profile.Credentials); syncErr != nil {
			log.Printf("kimi[%s] sync auth from %s: %v", req.ConversationID, containerName, syncErr)
		}
	}
	return err
}

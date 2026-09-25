package codex

import (
	"context"
	"log"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/codexharness"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

type Provider struct {
	projectPreparer       agent.ProjectPreparer
	credentialCollector   provisioning.CredentialCollector
	profile               provisioning.Profile
	credentialSyncTimeout time.Duration
	// accounts is nil when saved accounts are unavailable.
	accounts *agentauth.AccountService
}

func newProvider(
	projectPreparer agent.ProjectPreparer,
	credentialCollector provisioning.CredentialCollector,
	profile provisioning.Profile,
	credentialSyncTimeout time.Duration,
	accounts *agentauth.AccountService,
) *Provider {
	return &Provider{
		projectPreparer:       projectPreparer,
		credentialCollector:   credentialCollector,
		profile:               profile.Clone(),
		credentialSyncTimeout: credentialSyncTimeout,
		accounts:              accounts,
	}
}

func (p *Provider) ID() agent.ProviderID {
	return agent.ProviderCodex
}

func (p *Provider) Parser(req agent.RunRequest) agent.LineParser {
	return NewParser(req)
}

func (p *Provider) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	var run *accountRun
	var releaseAccount func()
	if p.accounts != nil {
		saved, isolated, err := p.accounts.CredentialForRun(req.AccountID)
		if err != nil {
			return err
		}
		if isolated {
			run, err = newAccountRun(req, saved)
			if err != nil {
				return err
			}
		} else {
			releaseAccount, err = p.accounts.BeginRunFor(ctx, req.AccountID)
			if err != nil {
				return err
			}
			defer releaseAccount()
		}
	}
	if emit == nil {
		emit = func(agent.Event) {}
	}
	if req.Provider == "" {
		req.Provider = agent.ProviderCodex
	}

	cmd, containerName, err := p.buildCmdForAccount(ctx, req, p.args(req), emit, run)
	if err != nil {
		return err
	}
	err = codexharness.Run(ctx, cmd, req, "Codex", emit)
	// An isolated account run refreshes only its private home. Pull that exact
	// home back from a project container, then offer it to the matching vault
	// record without changing another chat's credential files.
	if run != nil {
		credentialTouched := err == nil && containerName == ""
		if err == nil && containerName != "" && p.credentialCollector != nil {
			syncCtx, cancel := context.WithTimeout(context.Background(), p.credentialSyncTimeout)
			if syncErr := p.credentialCollector.SyncFromContainer(syncCtx, containerName, run.credentials); syncErr != nil {
				log.Printf("codex[%s] sync isolated auth from %s: %v", req.ConversationID, containerName, syncErr)
			}
			cancel()
			credentialTouched = true
		}
		if credentialTouched {
			credential, readErr := run.hostCredential()
			if readErr != nil {
				log.Printf("codex[%s] read isolated account login: %v", req.ConversationID, readErr)
			} else {
				captureCtx, cancel := context.WithTimeout(context.Background(), p.credentialSyncTimeout)
				if captureErr := p.accounts.CaptureRunCredential(captureCtx, run.saved, credential); captureErr != nil {
					log.Printf("codex[%s] keep isolated account login after run: %v", req.ConversationID, captureErr)
				}
				cancel()
			}
		}
		return err
	}

	// A successful legacy run may leave a refreshed login on the host, written by
	// the host CLI or copied back from the project container.
	hostLoginTouched := err == nil && containerName == ""
	if err == nil && containerName != "" && p.credentialCollector != nil {
		syncCtx, cancel := context.WithTimeout(context.Background(), p.credentialSyncTimeout)
		defer cancel()
		if syncErr := p.syncCredentialsFromContainer(syncCtx, containerName); syncErr != nil {
			log.Printf("codex[%s] sync auth from %s: %v", req.ConversationID, containerName, syncErr)
		}
		// Check the host login even when the copy failed: the container's
		// login may already have replaced it, as a rejected API-key login
		// does.
		hostLoginTouched = true
	}
	if hostLoginTouched && p.accounts != nil {
		captureCtx, cancel := context.WithTimeout(context.Background(), p.credentialSyncTimeout)
		defer cancel()
		if captureErr := p.accounts.CaptureAfterRun(captureCtx); captureErr != nil {
			log.Printf("codex[%s] keep account login after run: %v", req.ConversationID, captureErr)
		}
	}
	return err
}

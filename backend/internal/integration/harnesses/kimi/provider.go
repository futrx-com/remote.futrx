package kimi

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	harnessruntime "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/runtime"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

var errHeadlessPlanMode = errors.New("Kimi Plan mode requires an interactive terminal. Select Default for Remote chat or use Kimi in the workspace terminal")

type Provider struct {
	accountAuthorizer     harnessauth.AccountAuthorizer
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
	accountAuthorizer harnessauth.AccountAuthorizer,
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
	if err := harnessauth.RequireAccountUse(ctx, p.accountAuthorizer, harness.ProviderKimi, ""); err != nil {
		return err
	}
	if strings.TrimSpace(req.AccountID) != "" {
		return harnessauth.ErrAccountNotFound
	}
	if req.Mode == harness.RunModePlan {
		return errHeadlessPlanMode
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
	if err := harnessauth.RequireAccountUse(ctx, p.accountAuthorizer, harness.ProviderKimi, ""); err != nil {
		return err
	}
	err = harnessruntime.RunProcess(ctx, cmd, p.Parser(req), emit, harnessruntime.ProcessOptions{
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

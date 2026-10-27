package codex

import (
	"context"
	"errors"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

var ErrCodexAPIKeyAuth = errors.New("Codex is logged in with an API key; run codex login with ChatGPT to use subscription limits")

func validateSubscriptionCredentials(profile provisioning.Profile) error {
	if codexAuthUsesAPIKey(profile.Credentials.Files[0].HostPath) {
		return ErrCodexAPIKeyAuth
	}
	return nil
}

func (p *Provider) syncCredentialsFromContainer(ctx context.Context, containerName string) error {
	if err := p.credentialCollector.SyncFromContainer(ctx, containerName, p.profile.Credentials); err != nil {
		return err
	}
	if codexAuthUsesAPIKey(p.profile.Credentials.Files[0].HostPath) {
		return ErrCodexAPIKeyAuth
	}
	return nil
}

func codexAuthUsesAPIKey(path string) bool {
	_, usesAPIKey := codexAuthMode(path)
	return usesAPIKey
}

// Package launch coordinates best-effort capabilities after a new container
// is launched.
package launch

import "context"

type RegisteredCredentialEnsurer interface {
	EnsureRegistered(ctx context.Context, containerName string) error
}

type BrowserProvisioner interface {
	EnsureScript(ctx context.Context, containerName string) error
	EnsureSkill(ctx context.Context, containerName string) error
	EnsureNesting(ctx context.Context, containerName string) error
}

type WorkspaceProvisioner interface {
	EnsureSkillLinks(ctx context.Context, containerName string) error
}

// Provisioner applies launch-time capabilities in their stable order. Every
// step is deliberately best-effort so one unavailable capability cannot block
// the remaining migrations or the newly launched container.
type Provisioner struct {
	credentials RegisteredCredentialEnsurer
	workspace   WorkspaceProvisioner
	browser     BrowserProvisioner
}

func NewProvisioner(
	credentials RegisteredCredentialEnsurer,
	workspace WorkspaceProvisioner,
	browser BrowserProvisioner,
) *Provisioner {
	return &Provisioner{
		credentials: credentials,
		workspace:   workspace,
		browser:     browser,
	}
}

// Provision applies launch-time capabilities in their stable order.
func (p *Provisioner) Provision(ctx context.Context, containerName, _ string) {
	_ = p.credentials.EnsureRegistered(ctx, containerName)
	_ = p.workspace.EnsureSkillLinks(ctx, containerName)
	_ = p.browser.EnsureScript(ctx, containerName)
	_ = p.browser.EnsureSkill(ctx, containerName)
	_ = p.browser.EnsureNesting(ctx, containerName)
}

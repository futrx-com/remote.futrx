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
	EnsureGitIdentity(ctx context.Context, containerName string) error
}

type CodeServerProvisioner interface {
	Ensure(ctx context.Context, containerName, displayName, projectSlug string) error
}

type ScheduleToolsProvisioner interface {
	Ensure(ctx context.Context, containerName string) error
}

// Provisioner applies launch-time capabilities in their stable order. Every
// step is deliberately best-effort so one unavailable capability cannot block
// the remaining migrations or the newly launched container.
type Provisioner struct {
	credentials   RegisteredCredentialEnsurer
	workspace     WorkspaceProvisioner
	browser       BrowserProvisioner
	codeServer    CodeServerProvisioner
	scheduleTools ScheduleToolsProvisioner
}

func NewProvisioner(
	credentials RegisteredCredentialEnsurer,
	workspace WorkspaceProvisioner,
	browser BrowserProvisioner,
	codeServer CodeServerProvisioner,
	scheduleTools ...ScheduleToolsProvisioner,
) *Provisioner {
	var scheduled ScheduleToolsProvisioner
	if len(scheduleTools) > 0 {
		scheduled = scheduleTools[0]
	}
	return &Provisioner{
		credentials:   credentials,
		workspace:     workspace,
		browser:       browser,
		codeServer:    codeServer,
		scheduleTools: scheduled,
	}
}

// Provision applies launch-time capabilities in their stable order.
func (p *Provisioner) Provision(ctx context.Context, containerName, displayName, projectSlug string) {
	_ = p.credentials.EnsureRegistered(ctx, containerName)
	_ = p.workspace.EnsureSkillLinks(ctx, containerName)
	_ = p.workspace.EnsureGitIdentity(ctx, containerName)
	_ = p.browser.EnsureScript(ctx, containerName)
	_ = p.browser.EnsureSkill(ctx, containerName)
	_ = p.browser.EnsureNesting(ctx, containerName)
	if p.scheduleTools != nil {
		_ = p.scheduleTools.Ensure(ctx, containerName)
	}
	_ = p.codeServer.Ensure(ctx, containerName, displayName, projectSlug)
}

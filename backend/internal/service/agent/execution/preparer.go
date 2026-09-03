// Package execution owns provider-neutral project preparation for agent runs.
// Provider adapters declare the small policy differences and retain their own
// host commands, CLI arguments, stdin strategy, and output protocol.
package execution

import (
	"context"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

type Options struct {
	// Provider identifies system events and supplies default error wording.
	Provider agent.ProviderID
	// Profile is the exact provisioning policy already validated by the module
	// factory. New clones it before retaining any slices or template bytes.
	Profile provisioning.Profile
	// CLIErrorOperation and CredentialErrorOperation override the provider-based
	// default prefixes when compatibility requires established user-facing text.
	CLIErrorOperation        string
	CredentialErrorOperation string
	// BeforeCredentials receives an isolated profile snapshot for provider-specific
	// validation immediately before the generic credential synchronizer is invoked.
	BeforeCredentials func(provisioning.Profile) error
	// SkillLinksRequired turns the otherwise best-effort compatibility-link
	// migration into a fatal run prerequisite.
	SkillLinksRequired bool
	// BrowserAssets migrates the shared browser skill and script on every
	// prepared run. Both operations remain best effort.
	BrowserAssets bool
	// BrowserMCPRuntime provisions MCP configuration and starts browser core when
	// the already policy-gated run request enables Browser tools.
	BrowserMCPRuntime bool
}

type Preparer struct {
	projects   agent.ProjectResolver
	containers provisioning.ContainerDependencies
	options    Options
}

// New returns nil when no project resolver is available, allowing host-only
// runtime composition and focused tests without a project service.
func New(
	projects agent.ProjectResolver,
	containers provisioning.ContainerDependencies,
	options Options,
) agent.ProjectPreparer {
	if projects == nil {
		return nil
	}
	options.Profile = options.Profile.Clone()
	return &Preparer{projects: projects, containers: containers, options: options}
}

func (p *Preparer) Prepare(
	ctx context.Context,
	request agent.ProjectPreparationRequest,
	emit func(agent.Event),
) (agent.PreparedProject, error) {
	project, err := p.projects.Get(ctx, request.ProjectID)
	if err != nil {
		return agent.PreparedProject{}, fmt.Errorf("project not found (%s): %w", request.ProjectID, err)
	}
	if project.ContainerName == "" {
		return agent.PreparedProject{}, fmt.Errorf("project %s has no container - recreate the project", project.ID)
	}
	if project.Status != agent.ProjectStatusRunning {
		p.emitSystem(request, emit, "container_starting")
	}
	if _, err := p.projects.Start(ctx, project.ID); err != nil {
		return agent.PreparedProject{}, fmt.Errorf("start container: %w", err)
	}
	if err := p.containers.Validate(); err != nil {
		return agent.PreparedProject{}, err
	}
	if !p.containers.IsZero() {
		p.emitSystem(request, emit, "container_preparing")
		if err := p.prepareContainer(ctx, request, project.ContainerName); err != nil {
			return agent.PreparedProject{}, err
		}
	}

	prepared := agent.PreparedProject{ID: project.ID, ContainerName: project.ContainerName}
	if secrets, err := p.projects.ListSecrets(ctx, project.ID); err == nil {
		prepared.Secrets = secrets
	}
	return prepared, nil
}

func (p *Preparer) prepareContainer(
	ctx context.Context,
	request agent.ProjectPreparationRequest,
	containerName string,
) error {
	if err := p.containers.CLI.Ensure(ctx, containerName, p.options.Profile.CLI); err != nil {
		return fmt.Errorf("%s: %w", p.cliErrorOperation(), err)
	}
	if !p.options.Profile.Credentials.Empty() {
		if p.options.BeforeCredentials != nil {
			if err := p.options.BeforeCredentials(p.options.Profile.Clone()); err != nil {
				return fmt.Errorf("%s: %w", p.credentialErrorOperation(), err)
			}
		}
		if err := p.containers.Credentials.Ensure(ctx, containerName, p.options.Profile.Credentials); err != nil {
			return fmt.Errorf("%s: %w", p.credentialErrorOperation(), err)
		}
	}
	if err := p.containers.Workspace.EnsureAgentInstructions(ctx, containerName); err != nil {
		return fmt.Errorf("push agent instructions to container: %w", err)
	}
	if err := p.containers.Workspace.EnsureSkillLinks(ctx, containerName); err != nil && p.options.SkillLinksRequired {
		return fmt.Errorf("prepare workspace skill links: %w", err)
	}
	if p.options.BrowserAssets {
		_ = p.containers.Browser.EnsureSkill(ctx, containerName)
		_ = p.containers.Browser.EnsureScript(ctx, containerName)
	}
	if p.options.BrowserMCPRuntime && request.EnableBrowser {
		if err := p.containers.Browser.EnsureMCP(ctx, containerName); err != nil {
			return fmt.Errorf("provision browser MCP: %w", err)
		}
		if err := p.containers.Browser.EnsureCore(ctx, containerName); err != nil {
			return fmt.Errorf("start browser core: %w", err)
		}
	}
	if request.EnableScheduleTools {
		if err := p.containers.ScheduleTools.Ensure(ctx, containerName); err != nil {
			return fmt.Errorf("provision scheduled-task tools: %w", err)
		}
	}
	if err := p.containers.Lifecycle.EnsureBootAutostart(ctx, containerName); err != nil {
		return fmt.Errorf("set container boot.autostart: %w", err)
	}
	return nil
}

func (p *Preparer) cliErrorOperation() string {
	if p.options.CLIErrorOperation != "" {
		return p.options.CLIErrorOperation
	}
	return "install " + string(p.options.Provider) + " in container"
}

func (p *Preparer) credentialErrorOperation() string {
	if p.options.CredentialErrorOperation != "" {
		return p.options.CredentialErrorOperation
	}
	return "seed " + string(p.options.Provider) + " auth in container"
}

func (p *Preparer) emitSystem(
	request agent.ProjectPreparationRequest,
	emit func(agent.Event),
	subtype string,
) {
	if emit == nil {
		return
	}
	emit(agent.Event{
		T:              time.Now().UnixMilli(),
		Type:           agent.EventSystem,
		Provider:       p.options.Provider,
		ConversationID: request.ConversationID,
		Subtype:        subtype,
	})
}

package provisioning

import (
	"context"
	"fmt"
	"strings"
)

// CLIProvisioner is the agent-execution port for making an agent CLI
// available inside a project container.
type CLIProvisioner interface {
	Ensure(context.Context, string, CLISpec) error
}

// CredentialProvisioner is the shared-preparation port for seeding credentials
// into a project container before an agent starts.
type CredentialProvisioner interface {
	Ensure(context.Context, string, CredentialSpec) error
}

// CredentialCollector is the provider-facing port for retaining credentials
// that a successful project run may have refreshed inside its container.
type CredentialCollector interface {
	SyncFromContainer(context.Context, string, CredentialSpec) error
}

// CredentialSynchronizer combines the preparation and post-run roles for the
// concrete adapter wired by config. Consumers depend on the narrower role.
type CredentialSynchronizer interface {
	CredentialProvisioner
	CredentialCollector
}

// WorkspaceProvisioner publishes shared agent instructions and workspace links.
type WorkspaceProvisioner interface {
	EnsureAgentInstructions(context.Context, string) error
	EnsureSkillLinks(context.Context, string) error
}

// RuntimeAssetProvisioner publishes the selected provider's non-secret runtime
// assets inside a project container.
type RuntimeAssetProvisioner interface {
	Ensure(context.Context, string, []RuntimeAsset) error
}

// BrowserProvisioner publishes browser tooling and starts its shared core.
type BrowserProvisioner interface {
	EnsureSkill(context.Context, string) error
	EnsureScript(context.Context, string) error
	EnsureMCP(context.Context, string) error
	EnsureCore(context.Context, string) error
}

// ScheduleToolsProvisioner publishes the provider-neutral schedule CLI and
// its selected skill into a project workspace.
type ScheduleToolsProvisioner interface {
	Ensure(context.Context, string) error
}

// ContainerLifecycle owns lifecycle settings needed by agent runs.
type ContainerLifecycle interface {
	EnsureBootAutostart(context.Context, string) error
}

// ContainerDependencies groups the focused ports used by shared agent project
// preparation. A zero value lets focused/test composition reconcile a project
// while skipping the container-provisioning phase.
type ContainerDependencies struct {
	CLI           CLIProvisioner
	Credentials   CredentialSynchronizer
	Workspace     WorkspaceProvisioner
	RuntimeAssets RuntimeAssetProvisioner
	Browser       BrowserProvisioner
	ScheduleTools ScheduleToolsProvisioner
	Lifecycle     ContainerLifecycle
}

// IsZero reports whether no container provisioning ports were supplied.
func (d ContainerDependencies) IsZero() bool {
	return d.CLI == nil &&
		d.Credentials == nil &&
		d.Workspace == nil &&
		d.RuntimeAssets == nil &&
		d.Browser == nil &&
		d.ScheduleTools == nil &&
		d.Lifecycle == nil
}

// Validate accepts either that zero value or a complete set of container ports.
// Partial wiring is rejected before a preparation workflow can dereference a
// missing collaborator.
func (d ContainerDependencies) Validate() error {
	if d.IsZero() {
		return nil
	}

	missing := make([]string, 0, 7)
	if d.CLI == nil {
		missing = append(missing, "CLI")
	}
	if d.Credentials == nil {
		missing = append(missing, "credentials")
	}
	if d.Workspace == nil {
		missing = append(missing, "workspace")
	}
	if d.RuntimeAssets == nil {
		missing = append(missing, "runtime assets")
	}
	if d.Browser == nil {
		missing = append(missing, "browser")
	}
	if d.ScheduleTools == nil {
		missing = append(missing, "schedule tools")
	}
	if d.Lifecycle == nil {
		missing = append(missing, "lifecycle")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("incomplete container dependencies: missing %s", strings.Join(missing, ", "))
}

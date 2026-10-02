// Package inspection implements the LXD, guest, profile, and host-file probes
// used to assemble container diagnostic snapshots.
package inspection

import (
	"context"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/resources"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const inspectQuickTimeout = 5 * time.Second

// Adapter owns the independent best-effort probes backed by LXD, the guest
// operating system, configured agent profiles, and host credential files.
// Snapshot sequencing and lifecycle-state policy live in the application
// service.
type Adapter struct {
	resources   *resources.Manager
	lxd         containerLXDInspector
	guest       containerGuestInspector
	agents      containerAgentInspector
	credentials containerCredentialInspector
}

// NewAdapter returns independent diagnostic probes backed by the shared
// runtime and agent profiles.
func NewAdapter(
	runner command.Runner,
	profileSource serviceprofiles.Source,
	agentInstructions []byte,
) *Adapter {
	commands := &quickCommandRunner{runner: runner, timeout: inspectQuickTimeout}
	return &Adapter{
		resources: resources.NewManager(runner),
		lxd:       containerLXDInspector{commands: commands},
		guest:     containerGuestInspector{commands: commands},
		agents: containerAgentInspector{
			commands:        commands,
			profiles:        profileSource,
			instructionHash: assets.Hash(agentInstructions),
		},
		credentials: containerCredentialInspector{commands: commands, profiles: profileSource},
	}
}

// InspectConfiguration adds the provider-neutral LXD instance configuration
// fields available for a stopped or running container.
func (a *Adapter) InspectConfiguration(ctx context.Context, containerName string, out *serviceproject.ContainerInspect) {
	a.lxd.inspectConfiguration(ctx, containerName, out)
	cap, _, err := a.resources.DiskCapability(ctx, containerName)
	out.DiskQuota = &serviceproject.DiskQuotaInfo{Pool: cap.Pool, Driver: cap.Driver, Supported: cap.Supported, Detail: cap.Detail}
	if err != nil {
		out.DiskQuota.Detail = "storage quota capability unavailable"
	}
}

// InspectRuntime adds live LXD state and resource fields.
func (a *Adapter) InspectRuntime(ctx context.Context, containerName string, out *serviceproject.ContainerInspect) {
	a.lxd.inspectRuntime(ctx, containerName, out)
}

// InspectGuest reads operating-system and disk details from a running guest.
func (a *Adapter) InspectGuest(ctx context.Context, containerName string) (*serviceproject.OSInfo, []serviceproject.DiskUsage) {
	return a.guest.inspect(ctx, containerName)
}

// InspectAgents reports configured agent CLI and instruction readiness.
func (a *Adapter) InspectAgents(ctx context.Context, containerName string) []serviceproject.AgentContainerStatus {
	return a.agents.inspect(ctx, containerName)
}

// InspectCredentials compares host and guest credential file timestamps.
func (a *Adapter) InspectCredentials(
	ctx context.Context,
	containerName string,
	state serviceproject.ContainerState,
) []serviceproject.AuthBundleStatus {
	return a.credentials.inspect(ctx, containerName, state)
}

package config

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	containerbaseimage "github.com/futrx-com/remote.futrx.com/internal/integration/containers/baseimage"
	containerbrowser "github.com/futrx-com/remote.futrx.com/internal/integration/containers/browser"
	containercli "github.com/futrx-com/remote.futrx.com/internal/integration/containers/cli"
	containercodeserver "github.com/futrx-com/remote.futrx.com/internal/integration/containers/codeserver"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	containercredentials "github.com/futrx-com/remote.futrx.com/internal/integration/containers/credentials"
	containerenvironment "github.com/futrx-com/remote.futrx.com/internal/integration/containers/environment"
	containerinspection "github.com/futrx-com/remote.futrx.com/internal/integration/containers/inspection"
	containerlifecycle "github.com/futrx-com/remote.futrx.com/internal/integration/containers/lifecycle"
	containerlisteners "github.com/futrx-com/remote.futrx.com/internal/integration/containers/listeners"
	containernetwork "github.com/futrx-com/remote.futrx.com/internal/integration/containers/network"
	containerresources "github.com/futrx-com/remote.futrx.com/internal/integration/containers/resources"
	containerruntimeassets "github.com/futrx-com/remote.futrx.com/internal/integration/containers/runtimeassets"
	containerscheduletools "github.com/futrx-com/remote.futrx.com/internal/integration/containers/scheduletools"
	containerworkspace "github.com/futrx-com/remote.futrx.com/internal/integration/containers/workspace"
	"github.com/futrx-com/remote.futrx.com/internal/integration/hostfs"
	servicebrowser "github.com/futrx-com/remote.futrx.com/internal/service/container/browser"
	servicecli "github.com/futrx-com/remote.futrx.com/internal/service/container/cli"
	servicecredentials "github.com/futrx-com/remote.futrx.com/internal/service/container/credentials"
	serviceimage "github.com/futrx-com/remote.futrx.com/internal/service/container/image"
	serviceinspection "github.com/futrx-com/remote.futrx.com/internal/service/container/inspection"
	containerlaunch "github.com/futrx-com/remote.futrx.com/internal/service/container/launch"
	servicelifecycle "github.com/futrx-com/remote.futrx.com/internal/service/container/lifecycle"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const hostMappedUID = 1000000

// ContainerStack is the composition root for container application services
// and their LXD/host-filesystem adapters.
type ContainerStack struct {
	Lifecycle     *servicelifecycle.Service
	Inspection    *serviceinspection.Service
	Credentials   *servicecredentials.Service
	Environment   *containerenvironment.Client
	CLI           *servicecli.Provisioner
	Browser       *servicebrowser.Service
	ScheduleTools *containerscheduletools.Adapter
	Listeners     *containerlisteners.Scanner
	Network       *containernetwork.Repairer
	Workspace     *containerworkspace.Provisioner
	RuntimeAssets *containerruntimeassets.Adapter
	Images        *serviceimage.Builder
}

// ContainerStackOptions supplies presentation and installation-specific
// dependencies to the container composition root.
type ContainerStackOptions struct {
	AgentInstructions  []byte
	ImageBuildProgress serviceimage.ProgressReporter
}

// ProjectDependencies exposes only the capabilities consumed by project
// policy. Each capability can be replaced independently in tests or by a
// different runtime adapter.
func (s ContainerStack) ProjectDependencies() serviceproject.ContainerDependencies {
	return serviceproject.ContainerDependencies{
		Lifecycle:   s.Lifecycle,
		Environment: s.Environment,
		Inspector:   s.Inspection,
		Network:     s.Network,
		Listeners:   s.Listeners,
		Browser:     s.Browser,
	}
}

// AgentDependencies exposes only the capabilities used while preparing a
// container for an agent provider.
func (s ContainerStack) AgentDependencies() provisioning.ContainerDependencies {
	return provisioning.ContainerDependencies{
		CLI:           s.CLI,
		Credentials:   s.Credentials,
		Workspace:     s.Workspace,
		RuntimeAssets: s.RuntimeAssets,
		Browser:       s.Browser,
		ScheduleTools: s.ScheduleTools,
		Lifecycle:     s.Lifecycle,
	}
}

func NewContainerStack(
	runner command.Runner,
	configuredProfiles []provisioning.Profile,
	options ContainerStackOptions,
) ContainerStack {
	profiles := serviceprofiles.NewCatalog(configuredProfiles)
	publisher := assets.NewPublisher(runner)
	credentialTransfer := containercredentials.NewAdapter(runner)
	credentials := servicecredentials.NewService(profiles, credentialTransfer)
	environment := containerenvironment.NewClient(runner)
	listeners := containerlisteners.NewScanner(runner)
	network := containernetwork.NewRepairer(runner)
	cliRuntime := containercli.NewClient(runner)
	cli := servicecli.NewProvisioner(cliRuntime, profiles, serviceimage.InstallScript)
	browserAdapter := containerbrowser.NewAdapter(runner, profiles, publisher)
	browser := servicebrowser.NewService(servicebrowser.Dependencies{
		Provisioner: browserAdapter,
		Runtime:     browserAdapter,
		Tooling:     browserAdapter,
	}, containerbrowser.VNCPort)
	codeServer := containercodeserver.NewProvisioner(runner)
	scheduleTools := containerscheduletools.NewAdapter(runner, publisher)
	workspace := containerworkspace.NewProvisioner(
		runner,
		profiles,
		publisher,
		options.AgentInstructions,
	)
	runtimeAssets := containerruntimeassets.NewAdapter(runner, publisher)
	images := serviceimage.NewBuilder(
		containerbaseimage.NewClient(runner),
		profiles,
		containerbrowser.InstallScript(),
		containercodeserver.InstallScript(),
		options.ImageBuildProgress,
	)
	launchProvisioner := containerlaunch.NewProvisioner(
		credentials,
		workspace,
		browser,
		codeServer,
		scheduleTools,
	)
	resources := containerresources.NewManager(runner)
	lifecycle := servicelifecycle.NewService(
		containerlifecycle.NewClient(runner),
		serviceimage.Alias,
		hostfs.NewWorkspacePreparer(hostMappedUID, hostMappedUID),
		resources,
		launchProvisioner,
		profiles,
	)
	inspectionAdapter := containerinspection.NewAdapter(
		runner,
		profiles,
		options.AgentInstructions,
	)
	inspection := serviceinspection.NewService(serviceinspection.Dependencies{
		States:        lifecycle,
		Configuration: inspectionAdapter,
		Runtime:       inspectionAdapter,
		Guest:         inspectionAdapter,
		Agents:        inspectionAdapter,
		Credentials:   inspectionAdapter,
	})

	return ContainerStack{
		Lifecycle:     lifecycle,
		Inspection:    inspection,
		Credentials:   credentials,
		Environment:   environment,
		CLI:           cli,
		Browser:       browser,
		ScheduleTools: scheduleTools,
		Listeners:     listeners,
		Network:       network,
		Workspace:     workspace,
		RuntimeAssets: runtimeAssets,
		Images:        images,
	}
}

// Package module defines the provider-neutral contract implemented by every
// agent integration. A module combines static behavior metadata with the
// factory that creates its runtime provider and optional auth binding.
package module

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentexecution "github.com/futrx-com/remote.futrx.com/internal/service/agent/execution"
)

var (
	ErrInvalidFactory = errors.New("invalid agent module factory")
)

type ExecutionScope string

const (
	ScopeHost    ExecutionScope = "host"
	ScopeProject ExecutionScope = "project"
)

type AuthMode string

const (
	AuthManagedCode   AuthMode = "managed-code"
	AuthManagedDevice AuthMode = "managed-device"
	AuthExternal      AuthMode = "external"
	AuthNone          AuthMode = "none"
)

type SkillStrategy string

const (
	SkillsNone          SkillStrategy = "none"
	SkillsSlashCommand  SkillStrategy = "slash-command"
	SkillsDollarMention SkillStrategy = "dollar-mention"
	SkillsInstructions  SkillStrategy = "instructions"
)

// SessionSupport describes which provider-native conversation operations the
// orchestration layer may use. Fork implies resume support.
type SessionSupport struct {
	Resume bool
	Fork   bool
}

// Features describes optional behavior the platform may expose for an agent.
// The provider still owns the concrete CLI flags and protocol translation.
type Features struct {
	Sessions       SessionSupport
	Skills         SkillStrategy
	BrowserTools   bool
	ScheduledTools bool
}

// Descriptor is the stable, provider-neutral declaration consumed by runtime
// policy and presentation. Installation and filesystem policy is deliberately
// kept in the factory's separate provisioning profile.
type Descriptor struct {
	ID                  agent.ProviderID
	Label               string
	Default             bool
	ExecutionScopes     []ExecutionScope
	Auth                AuthMode
	AuthInstructions    string
	SatisfiesAccessGate bool
	LegacySkillRoots    []string
	Features            Features
}

// BuildDependencies are application-owned collaborators supplied once when a
// catalog builds its runtime. Factory owns their provider-specific projection.
type BuildDependencies struct {
	Projects              agent.ProjectResolver
	Containers            provisioning.ContainerDependencies
	CredentialSyncTimeout time.Duration
}

// Dependencies are the already-projected collaborators exposed to one
// provider build callback. Project adapters receive a shared preparer rather
// than the resolver and provisioning workflow separately.
type Dependencies struct {
	ProjectPreparer       agent.ProjectPreparer
	CredentialCollector   provisioning.CredentialCollector
	CredentialSyncTimeout time.Duration
}

// ProjectPreparationPolicy declares the few provider-specific choices in the
// otherwise shared project lifecycle and provisioning workflow.
type ProjectPreparationPolicy struct {
	// CLIErrorOperation and CredentialErrorOperation retain established
	// provider-specific error prefixes instead of the shared defaults.
	CLIErrorOperation        string
	CredentialErrorOperation string
	// BeforeCredentials receives an isolated profile snapshot immediately before
	// the factory-owned preparer invokes the generic credential provisioner.
	BeforeCredentials func(provisioning.Profile) error
	// SkillLinksRequired makes compatibility-link migration fatal. It is best
	// effort by default.
	SkillLinksRequired bool
	// BrowserAssets migrates shared browser files on every prepared run. The
	// migration remains best effort.
	BrowserAssets bool
	// BrowserMCPRuntime provisions MCP configuration and starts browser core when
	// the already policy-gated request enables browser tools.
	BrowserMCPRuntime bool
}

// Components are the runtime objects produced by a module. Auth is nil only
// when the descriptor explicitly declares AuthNone.
type Components struct {
	Provider agent.Provider
	Auth     *agentauth.Binding
}

// BuildFunc creates one provider runtime from projected provider dependencies
// and the exact provisioning profile already validated by the factory. The
// profile is nil only for a host-only module with no local CLI installation.
type BuildFunc func(Dependencies, *provisioning.Profile) (Components, error)

// FactoryBuilder is the compile-time constructor contract implemented by each
// provider package and consumed by the explicit config composition root.
type FactoryBuilder func() (Factory, error)

// Factory is immutable after construction. Descriptor and provisioning profile
// data are cloned at the boundary so callers cannot mutate registered policy.
type Factory struct {
	descriptor         Descriptor
	profile            *provisioning.Profile
	projectPreparation ProjectPreparationPolicy
	build              BuildFunc
}

// FactoryOption declares optional provider policy without exposing mutable
// factory state to provider packages.
type FactoryOption func(*Factory) error

// WithProjectPreparation declares provider-specific choices applied when the
// factory constructs its shared ProjectPreparer. Project modules receive the
// common default policy when this option is omitted.
func WithProjectPreparation(policy ProjectPreparationPolicy) FactoryOption {
	return func(factory *Factory) error {
		factory.projectPreparation = policy
		return nil
	}
}

func NewFactory(
	descriptor Descriptor,
	profile *provisioning.Profile,
	build BuildFunc,
	options ...FactoryOption,
) (Factory, error) {
	factory := Factory{
		descriptor: cloneDescriptor(descriptor),
		profile:    cloneProfile(profile),
		build:      build,
	}
	for _, option := range options {
		if option == nil {
			return Factory{}, fmt.Errorf("%w: provider %q has a nil option", ErrInvalidFactory, factory.descriptor.ID)
		}
		if err := option(&factory); err != nil {
			return Factory{}, fmt.Errorf("%w: provider %q option: %v", ErrInvalidFactory, factory.descriptor.ID, err)
		}
	}
	if err := validateDescriptor(factory.descriptor, factory.profile); err != nil {
		return Factory{}, err
	}
	if err := validateProjectPreparation(factory.descriptor, factory.profile, factory.projectPreparation); err != nil {
		return Factory{}, err
	}
	if factory.build == nil {
		return Factory{}, fmt.Errorf("%w: provider %q has no builder", ErrInvalidFactory, factory.descriptor.ID)
	}
	return factory, nil
}

func (f Factory) Descriptor() Descriptor {
	return cloneDescriptor(f.descriptor)
}

func (f Factory) buildComponents(deps BuildDependencies) (Components, error) {
	if f.build == nil {
		return Components{}, fmt.Errorf("%w: provider %q has no builder", ErrInvalidFactory, f.descriptor.ID)
	}
	var profile *provisioning.Profile
	if f.profile != nil {
		cloned := f.profile.Clone()
		profile = &cloned
	}
	providerDependencies := Dependencies{
		CredentialCollector:   deps.Containers.Credentials,
		CredentialSyncTimeout: deps.CredentialSyncTimeout,
	}
	if supportsExecutionScope(f.descriptor, ScopeProject) {
		providerDependencies.ProjectPreparer = agentexecution.New(
			deps.Projects,
			deps.Containers,
			agentexecution.Options{
				Provider:                 f.descriptor.ID,
				Profile:                  *profile,
				CLIErrorOperation:        f.projectPreparation.CLIErrorOperation,
				CredentialErrorOperation: f.projectPreparation.CredentialErrorOperation,
				BeforeCredentials:        f.projectPreparation.BeforeCredentials,
				SkillLinksRequired:       f.projectPreparation.SkillLinksRequired,
				BrowserAssets:            f.projectPreparation.BrowserAssets,
				BrowserMCPRuntime:        f.projectPreparation.BrowserMCPRuntime,
			},
		)
	}
	components, err := f.build(providerDependencies, profile)
	if err != nil {
		return Components{}, fmt.Errorf("build agent %q: %w", f.descriptor.ID, err)
	}
	if components.Provider == nil {
		return Components{}, fmt.Errorf("%w: provider %q builder returned nil", ErrInvalidFactory, f.descriptor.ID)
	}
	if components.Provider.ID() != f.descriptor.ID {
		return Components{}, fmt.Errorf(
			"%w: descriptor %q built provider %q",
			ErrInvalidFactory,
			f.descriptor.ID,
			components.Provider.ID(),
		)
	}
	if err := validateAuth(f.descriptor, components.Auth); err != nil {
		return Components{}, err
	}
	return components, nil
}

func validateProjectPreparation(
	descriptor Descriptor,
	profile *provisioning.Profile,
	policy ProjectPreparationPolicy,
) error {
	projectScoped := supportsExecutionScope(descriptor, ScopeProject)
	if !projectScoped && hasProjectPreparationPolicy(policy) {
		return fmt.Errorf("%w: host-only provider %q declares project preparation", ErrInvalidFactory, descriptor.ID)
	}
	if !projectScoped {
		return nil
	}
	if policy.BrowserMCPRuntime && !descriptor.Features.BrowserTools {
		return fmt.Errorf("%w: provider %q prepares browser MCP without browser support", ErrInvalidFactory, descriptor.ID)
	}
	if profile != nil && profile.Credentials.Empty() &&
		(policy.CredentialErrorOperation != "" || policy.BeforeCredentials != nil) {
		return fmt.Errorf("%w: provider %q customizes credentials without credential policy", ErrInvalidFactory, descriptor.ID)
	}
	return nil
}

func hasProjectPreparationPolicy(policy ProjectPreparationPolicy) bool {
	return policy.CLIErrorOperation != "" ||
		policy.CredentialErrorOperation != "" ||
		policy.BeforeCredentials != nil ||
		policy.SkillLinksRequired ||
		policy.BrowserAssets ||
		policy.BrowserMCPRuntime
}

func supportsExecutionScope(descriptor Descriptor, scope ExecutionScope) bool {
	for _, candidate := range descriptor.ExecutionScopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func validateDescriptor(descriptor Descriptor, profile *provisioning.Profile) error {
	id := string(descriptor.ID)
	if !agent.ValidProviderID(descriptor.ID) {
		return fmt.Errorf("%w: provider ID %q is invalid", ErrInvalidFactory, id)
	}
	if strings.TrimSpace(descriptor.Label) == "" {
		return fmt.Errorf("%w: provider %q has no label", ErrInvalidFactory, descriptor.ID)
	}
	if err := validateScopes(descriptor, profile); err != nil {
		return err
	}
	if err := validateAuthMode(descriptor); err != nil {
		return err
	}
	if descriptor.Features.Sessions.Fork && !descriptor.Features.Sessions.Resume {
		return fmt.Errorf("%w: provider %q declares fork without resume", ErrInvalidFactory, descriptor.ID)
	}
	switch descriptor.Features.Skills {
	case SkillsNone, SkillsSlashCommand, SkillsDollarMention, SkillsInstructions:
	default:
		return fmt.Errorf("%w: provider %q has unknown skill strategy %q", ErrInvalidFactory, descriptor.ID, descriptor.Features.Skills)
	}
	if descriptor.Features.Skills == SkillsNone && len(descriptor.LegacySkillRoots) > 0 {
		return fmt.Errorf("%w: provider %q has skill roots but disables skills", ErrInvalidFactory, descriptor.ID)
	}
	for _, root := range descriptor.LegacySkillRoots {
		if strings.TrimSpace(root) == "" {
			return fmt.Errorf("%w: provider %q has an empty legacy skill root", ErrInvalidFactory, descriptor.ID)
		}
	}
	return nil
}

func validateScopes(descriptor Descriptor, profile *provisioning.Profile) error {
	if len(descriptor.ExecutionScopes) == 0 {
		return fmt.Errorf("%w: provider %q has no execution scope", ErrInvalidFactory, descriptor.ID)
	}
	seen := make(map[ExecutionScope]bool, len(descriptor.ExecutionScopes))
	projectScoped := false
	for _, scope := range descriptor.ExecutionScopes {
		if scope != ScopeHost && scope != ScopeProject {
			return fmt.Errorf("%w: provider %q has unknown execution scope %q", ErrInvalidFactory, descriptor.ID, scope)
		}
		if seen[scope] {
			return fmt.Errorf("%w: provider %q repeats execution scope %q", ErrInvalidFactory, descriptor.ID, scope)
		}
		seen[scope] = true
		projectScoped = projectScoped || scope == ScopeProject
	}
	if descriptor.Default && !seen[ScopeHost] {
		return fmt.Errorf("%w: default provider %q does not support host execution", ErrInvalidFactory, descriptor.ID)
	}
	if projectScoped && profile == nil {
		return fmt.Errorf("%w: project provider %q has no profile", ErrInvalidFactory, descriptor.ID)
	}
	if profile != nil {
		return validateProfile(descriptor.ID, profile, projectScoped)
	}
	return nil
}

func validateProfile(id agent.ProviderID, profile *provisioning.Profile, projectScoped bool) error {
	if profile.ID != string(id) {
		return fmt.Errorf("%w: provider %q has profile %q", ErrInvalidFactory, id, profile.ID)
	}
	cli := profile.CLI
	if strings.TrimSpace(cli.Name) == "" || strings.TrimSpace(cli.Binary) == "" || strings.TrimSpace(cli.Version) == "" {
		return fmt.Errorf("%w: provider %q has incomplete CLI policy", ErrInvalidFactory, id)
	}
	if !provisioning.ValidCLIVersion(cli.Version) {
		return fmt.Errorf("%w: provider %q has invalid CLI version %q", ErrInvalidFactory, id, cli.Version)
	}
	if cli.InstallTimeout <= 0 {
		return fmt.Errorf("%w: provider %q has a non-positive CLI install timeout", ErrInvalidFactory, id)
	}
	if cli.WaitTimeout <= 0 {
		return fmt.Errorf("%w: provider %q has a non-positive CLI wait timeout", ErrInvalidFactory, id)
	}
	if (cli.CheckVersion || cli.ReportVersion) && len(cli.VersionArgs) == 0 {
		return fmt.Errorf("%w: provider %q has no CLI version arguments", ErrInvalidFactory, id)
	}
	for _, argument := range cli.VersionArgs {
		if strings.TrimSpace(argument) == "" || strings.ContainsRune(argument, '\x00') {
			return fmt.Errorf("%w: provider %q has an invalid CLI version argument", ErrInvalidFactory, id)
		}
	}
	if projectScoped && strings.TrimSpace(cli.ImageLabel) == "" {
		return fmt.Errorf("%w: project provider %q has no image label", ErrInvalidFactory, id)
	}
	switch cli.InstallMode {
	case provisioning.InstallWithNPM, provisioning.InstallWithImageRepair:
		if strings.TrimSpace(cli.PackageName) == "" {
			return fmt.Errorf("%w: provider %q has no CLI package", ErrInvalidFactory, id)
		}
	case provisioning.InstallWithScript:
		if strings.TrimSpace(cli.InstallScript) == "" {
			return fmt.Errorf("%w: provider %q has no install script", ErrInvalidFactory, id)
		}
	default:
		return fmt.Errorf("%w: provider %q has unknown install mode %q", ErrInvalidFactory, id, cli.InstallMode)
	}
	seenDevices := make(map[string]bool, len(profile.PersistentState))
	seenHosts := make(map[string]bool, len(profile.PersistentState))
	seenTargets := make(map[string]bool, len(profile.PersistentState))
	for _, state := range profile.PersistentState {
		if err := state.Validate(); err != nil {
			return fmt.Errorf("%w: provider %q has invalid persistent state: %v", ErrInvalidFactory, id, err)
		}
		if seenDevices[state.Device] || seenHosts[state.HostDirectory] || seenTargets[state.ContainerPath] {
			return fmt.Errorf("%w: provider %q repeats a persistent-state mount", ErrInvalidFactory, id)
		}
		seenDevices[state.Device] = true
		seenHosts[state.HostDirectory] = true
		seenTargets[state.ContainerPath] = true
	}
	return nil
}

func validateAuthMode(descriptor Descriptor) error {
	switch descriptor.Auth {
	case AuthManagedCode, AuthManagedDevice, AuthExternal:
		if strings.TrimSpace(descriptor.AuthInstructions) == "" {
			return fmt.Errorf("%w: provider %q auth has no instructions", ErrInvalidFactory, descriptor.ID)
		}
		if descriptor.Auth == AuthExternal && descriptor.SatisfiesAccessGate {
			return fmt.Errorf("%w: provider %q external auth cannot satisfy the access gate", ErrInvalidFactory, descriptor.ID)
		}
	case AuthNone:
		if strings.TrimSpace(descriptor.AuthInstructions) != "" {
			return fmt.Errorf("%w: provider %q has instructions for auth mode %q", ErrInvalidFactory, descriptor.ID, descriptor.Auth)
		}
	default:
		return fmt.Errorf("%w: provider %q has unknown auth mode %q", ErrInvalidFactory, descriptor.ID, descriptor.Auth)
	}
	return nil
}

func validateAuth(descriptor Descriptor, binding *agentauth.Binding) error {
	if descriptor.Auth == AuthNone {
		if binding != nil {
			return fmt.Errorf("%w: provider %q declares no auth but built a binding", ErrInvalidFactory, descriptor.ID)
		}
		return nil
	}
	if binding == nil {
		return fmt.Errorf("%w: provider %q did not build an auth binding", ErrInvalidFactory, descriptor.ID)
	}
	if binding.ID() != descriptor.ID {
		return fmt.Errorf("%w: provider %q built auth binding %q", ErrInvalidFactory, descriptor.ID, binding.ID())
	}
	wantFlow := map[AuthMode]agentauth.Flow{
		AuthManagedCode:   agentauth.FlowCode,
		AuthManagedDevice: agentauth.FlowDevice,
		AuthExternal:      agentauth.FlowExternal,
	}[descriptor.Auth]
	if binding.Flow() != wantFlow {
		return fmt.Errorf("%w: provider %q auth flow is %q, want %q", ErrInvalidFactory, descriptor.ID, binding.Flow(), wantFlow)
	}
	if descriptor.Auth == AuthExternal && binding.Available() {
		return fmt.Errorf("%w: provider %q external auth exposes a managed service", ErrInvalidFactory, descriptor.ID)
	}
	if descriptor.Auth != AuthExternal && !binding.Available() {
		return fmt.Errorf("%w: provider %q managed auth is unavailable", ErrInvalidFactory, descriptor.ID)
	}
	return nil
}

func cloneDescriptor(descriptor Descriptor) Descriptor {
	descriptor.ExecutionScopes = append([]ExecutionScope(nil), descriptor.ExecutionScopes...)
	descriptor.LegacySkillRoots = append([]string(nil), descriptor.LegacySkillRoots...)
	return descriptor
}

func cloneProfile(profile *provisioning.Profile) *provisioning.Profile {
	if profile == nil {
		return nil
	}
	cloned := profile.Clone()
	return &cloned
}

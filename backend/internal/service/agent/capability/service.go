// Package capability aggregates the provider-specific capability catalogs
// exposed by the registered agent CLIs.
//
// Capability discovery may start several comparatively expensive CLI probes (for
// example, Codex app-server and provider model-list commands). Simultaneous
// requests for the same host or project container share one in-flight probe.
// Completed catalogs are cached per execution environment using the healthy
// and degraded TTLs supplied by application configuration. A manual refresh
// bypasses a completed cache entry and starts or joins one discovery flight
// whose result replaces it. A backend restart clears every entry.
package capability

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

var (
	ErrProjectLookupUnavailable = errors.New("project lookup unavailable")
	ErrProjectNotFound          = errors.New("project not found")
	ErrAuthenticationRequired   = errors.New("authentication required")
	ErrProjectAccessDenied      = errors.New("project access denied")
)

type ProjectCatalog interface {
	Get(ctx context.Context, id serviceproject.ID) (serviceproject.Meta, error)
	HasAccess(ctx context.Context, id serviceproject.ID, email string) (bool, error)
}

type Authorizer interface {
	CurrentSession(ctx context.Context, cookieValue string) (*serviceauth.Session, error)
	IsAdmin(ctx context.Context, email string) (bool, error)
}

type CapabilityRegistry interface {
	CapabilityProviders() []agent.CapabilityProvider
}

type ScopePolicy interface {
	SupportsScope(provider string, scope agentmodule.ExecutionScope) bool
}

type DescriptorPolicy interface {
	Descriptor(provider string) (agentmodule.Descriptor, bool)
}

type ListQuery struct {
	ProjectID     serviceproject.ID
	SessionCookie string
	Refresh       bool
}

type Service struct {
	agents            CapabilityRegistry
	projects          ProjectCatalog
	auth              Authorizer
	capabilityTimeout time.Duration
	scopes            ScopePolicy
	descriptors       DescriptorPolicy
	cache             *catalogCache
	flights           *catalogFlights
}

// Settings are cross-provider discovery policies supplied by the application
// composition root. Provider-specific probe behavior remains in each adapter.
type Settings struct {
	CapabilityTimeout          time.Duration
	CapabilityCacheTTL         time.Duration
	DegradedCapabilityCacheTTL time.Duration
}

func WithScopePolicy(policy ScopePolicy) Option {
	return func(catalog *Service) {
		catalog.scopes = policy
	}
}

type ModulePolicy interface {
	ScopePolicy
	DescriptorPolicy
}

// WithModulePolicy applies the same validated module declarations to scope
// filtering and public capability metadata.
func WithModulePolicy(policy ModulePolicy) Option {
	return func(target *Service) {
		target.scopes = policy
		target.descriptors = policy
	}
}

type Option func(*Service)

func New(
	agents CapabilityRegistry,
	projects ProjectCatalog,
	auth Authorizer,
	settings Settings,
	options ...Option,
) *Service {
	catalog := &Service{
		agents:            agents,
		projects:          projects,
		auth:              auth,
		capabilityTimeout: settings.CapabilityTimeout,
		cache: newCatalogCache(
			settings.CapabilityCacheTTL,
			settings.DegradedCapabilityCacheTTL,
		),
		flights: newCatalogFlights(),
	}
	for _, option := range options {
		if option != nil {
			option(catalog)
		}
	}
	return catalog
}

func (c *Service) List(ctx context.Context, query ListQuery) ([]agent.Capabilities, error) {
	containerName := ""
	flightKey := "host"
	scope := agentmodule.ScopeHost
	if query.ProjectID != "" {
		if c.projects == nil {
			return nil, ErrProjectLookupUnavailable
		}
		project, err := c.projects.Get(ctx, query.ProjectID)
		if err != nil {
			if errors.Is(err, serviceproject.ErrNotFound) {
				return nil, ErrProjectNotFound
			}
			return nil, err
		}
		if err := c.authorize(ctx, project.ID, query.SessionCookie); err != nil {
			return nil, err
		}
		containerName = project.ContainerName
		flightKey = "project:" + string(project.ID) + ":" + containerName
		scope = agentmodule.ScopeProject
	}

	if !query.Refresh {
		if cached, ok := c.cache.load(flightKey); ok {
			return cached, nil
		}
	}

	return c.flights.do(ctx, flightKey, func(discoveryCtx context.Context) ([]agent.Capabilities, error) {
		// A catalog may have completed between the optimistic cache check and
		// this caller becoming the flight leader.
		if !query.Refresh {
			if cached, ok := c.cache.load(flightKey); ok {
				return cached, nil
			}
		}
		providers := c.capabilityProviders(scope)
		result := make([]agent.Capabilities, len(providers))
		var wait sync.WaitGroup
		for index, provider := range providers {
			wait.Add(1)
			go func() {
				defer wait.Done()
				probeCtx := discoveryCtx
				cancel := func() {}
				if c.capabilityTimeout > 0 {
					probeCtx, cancel = context.WithTimeout(discoveryCtx, c.capabilityTimeout)
				}
				defer cancel()
				caps, err := provider.Capabilities(
					probeCtx,
					agent.CapabilityRequest{ContainerName: containerName},
				)
				caps.Provider = provider.ID()
				c.decorate(&caps)
				if caps.Source == "" {
					caps.Source = agent.CapabilitySourceFallback
				}
				if err != nil && caps.Warning == "" {
					caps.Warning = "Provider capabilities are temporarily unavailable"
				}
				if caps.Models == nil {
					caps.Models = []agent.ModelCapability{}
				}
				if caps.Modes == nil {
					caps.Modes = []agent.CapabilityOption{}
				}
				result[index] = caps
			}()
		}
		wait.Wait()
		c.cache.store(flightKey, result)
		return result, nil
	})
}

func (c *Service) decorate(capabilities *agent.Capabilities) {
	if capabilities == nil || c.descriptors == nil {
		return
	}
	descriptor, ok := c.descriptors.Descriptor(string(capabilities.Provider))
	if !ok {
		return
	}
	capabilities.Label = descriptor.Label
	capabilities.Default = descriptor.Default
	capabilities.ExecutionScopes = make([]string, len(descriptor.ExecutionScopes))
	for index, scope := range descriptor.ExecutionScopes {
		capabilities.ExecutionScopes[index] = string(scope)
	}
	capabilities.Authentication = agent.CapabilityAuthentication{
		Mode:                string(descriptor.Auth),
		Instructions:        descriptor.AuthInstructions,
		SatisfiesAccessGate: descriptor.SatisfiesAccessGate,
	}
	capabilities.Features = agent.CapabilityFeatures{
		Sessions: agent.CapabilitySessionSupport{
			Resume: descriptor.Features.Sessions.Resume,
			Fork:   descriptor.Features.Sessions.Fork,
		},
		Skills:         string(descriptor.Features.Skills),
		BrowserTools:   descriptor.Features.BrowserTools,
		ScheduledTools: descriptor.Features.ScheduledTools,
	}
}

func (c *Service) capabilityProviders(scope agentmodule.ExecutionScope) []agent.CapabilityProvider {
	providers := c.agents.CapabilityProviders()
	if c.scopes == nil {
		return providers
	}
	filtered := make([]agent.CapabilityProvider, 0, len(providers))
	for _, provider := range providers {
		if c.scopes.SupportsScope(string(provider.ID()), scope) {
			filtered = append(filtered, provider)
		}
	}
	return filtered
}

func (c *Service) authorize(ctx context.Context, projectID serviceproject.ID, cookie string) error {
	if c.auth == nil {
		return nil
	}
	session, err := c.auth.CurrentSession(ctx, cookie)
	if err != nil || session == nil {
		return ErrAuthenticationRequired
	}
	email := strings.ToLower(strings.TrimSpace(session.Email))
	if email == "" {
		return ErrAuthenticationRequired
	}
	isAdmin, _ := c.auth.IsAdmin(ctx, email)
	if isAdmin {
		return nil
	}
	hasAccess, err := c.projects.HasAccess(ctx, projectID, email)
	if err != nil {
		return err
	}
	if !hasAccess {
		return ErrProjectAccessDenied
	}
	return nil
}

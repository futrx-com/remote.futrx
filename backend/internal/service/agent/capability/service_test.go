package capability

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type catalogTestProvider struct {
	id         agent.ProviderID
	reportedID agent.ProviderID
	label      string
	mu         sync.Mutex
	calls      int
	requests   []agent.CapabilityRequest
	entered    chan<- struct{}
	release    <-chan struct{}
}

func (p *catalogTestProvider) ID() agent.ProviderID { return p.id }
func (p *catalogTestProvider) Capabilities(ctx context.Context, req agent.CapabilityRequest) (agent.Capabilities, error) {
	p.mu.Lock()
	p.calls++
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	if p.entered != nil {
		p.entered <- struct{}{}
	}
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return agent.Capabilities{Provider: p.id, Label: p.label}, ctx.Err()
		}
	}
	return agent.Capabilities{
		Provider: firstProviderID(p.reportedID, p.id), Label: p.label, Source: agent.CapabilitySourceLive,
		Models: []agent.ModelCapability{}, Modes: []agent.CapabilityOption{},
	}, nil
}

func firstProviderID(values ...agent.ProviderID) agent.ProviderID {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func TestListAppliesOneConfiguredTimeoutPerProvider(t *testing.T) {
	provider := &catalogTestProvider{
		id:      "slow-agent",
		label:   "Slow Agent",
		release: make(chan struct{}),
	}
	catalog := New(
		catalogTestRegistry{providers: []agent.CapabilityProvider{provider}},
		nil,
		nil,
		Settings{
			CapabilityTimeout:          20 * time.Millisecond,
			CapabilityCacheTTL:         24 * time.Hour,
			DegradedCapabilityCacheTTL: 2 * time.Hour,
		},
	)

	started := time.Now()
	items, err := catalog.List(context.Background(), ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("capability discovery took %s despite configured timeout", elapsed)
	}
	if len(items) != 1 || items[0].Source != agent.CapabilitySourceFallback || items[0].Warning == "" {
		t.Fatalf("timed-out capabilities = %#v", items)
	}
}

type catalogTestRegistry struct {
	providers []agent.CapabilityProvider
}

func (r catalogTestRegistry) CapabilityProviders() []agent.CapabilityProvider {
	return r.providers
}

type catalogTestProjects struct {
	project serviceproject.Meta
}

type catalogTestScopes map[agent.ProviderID]map[agentmodule.ExecutionScope]bool

func (s catalogTestScopes) SupportsScope(provider string, scope agentmodule.ExecutionScope) bool {
	return s[agent.ProviderID(provider)][scope]
}

type catalogTestModules map[agent.ProviderID]agentmodule.Descriptor

func (m catalogTestModules) Descriptor(provider string) (agentmodule.Descriptor, bool) {
	descriptor, ok := m[agent.ProviderID(provider)]
	return descriptor, ok
}

func (m catalogTestModules) SupportsScope(provider string, scope agentmodule.ExecutionScope) bool {
	descriptor, ok := m[agent.ProviderID(provider)]
	if !ok {
		return false
	}
	for _, configured := range descriptor.ExecutionScopes {
		if configured == scope {
			return true
		}
	}
	return false
}

func (p catalogTestProjects) Get(context.Context, serviceproject.ID) (serviceproject.Meta, error) {
	return p.project, nil
}

func (catalogTestProjects) HasAccess(context.Context, serviceproject.ID, string) (bool, error) {
	return true, nil
}

func TestListUsesRegistryOrderProjectContainerAndSharedCache(t *testing.T) {
	claude := &catalogTestProvider{id: agent.ProviderClaude, label: "Claude"}
	codex := &catalogTestProvider{id: agent.ProviderCodex, label: "Codex"}
	registry := catalogTestRegistry{providers: []agent.CapabilityProvider{claude, codex}}
	catalog := New(registry, catalogTestProjects{project: serviceproject.Meta{
		ID: "abcd", ContainerName: "remote-abcd", Status: serviceproject.StatusRunning,
	}}, nil, testSettings())

	for call := 0; call < 2; call++ {
		items, err := catalog.List(context.Background(), ListQuery{ProjectID: "abcd"})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 || items[0].Provider != agent.ProviderClaude || items[1].Provider != agent.ProviderCodex {
			t.Fatalf("items = %+v", items)
		}
	}
	if claude.calls != 1 || codex.calls != 1 {
		t.Fatalf("capability calls = claude:%d codex:%d", claude.calls, codex.calls)
	}
	if got := claude.requests[0].ContainerName; got != "remote-abcd" {
		t.Fatalf("container name = %q", got)
	}

	if _, err := catalog.List(context.Background(), ListQuery{ProjectID: "abcd", Refresh: true}); err != nil {
		t.Fatal(err)
	}
	if claude.calls != 2 || codex.calls != 2 {
		t.Fatalf("refreshed capability calls = claude:%d codex:%d", claude.calls, codex.calls)
	}
}

func TestListOnlyDiscoversProvidersDeclaredForRequestedScope(t *testing.T) {
	host := &catalogTestProvider{id: "host-agent", label: "Host"}
	project := &catalogTestProvider{id: "project-agent", label: "Project"}
	catalog := New(
		catalogTestRegistry{providers: []agent.CapabilityProvider{host, project}},
		catalogTestProjects{project: serviceproject.Meta{ID: "abcd", ContainerName: "remote-abcd"}},
		nil,
		testSettings(),
		WithScopePolicy(catalogTestScopes{
			"host-agent":    {agentmodule.ScopeHost: true},
			"project-agent": {agentmodule.ScopeProject: true},
		}),
	)

	hostItems, err := catalog.List(context.Background(), ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hostItems) != 1 || hostItems[0].Provider != "host-agent" {
		t.Fatalf("host capabilities = %#v", hostItems)
	}
	projectItems, err := catalog.List(context.Background(), ListQuery{ProjectID: "abcd"})
	if err != nil {
		t.Fatal(err)
	}
	if len(projectItems) != 1 || projectItems[0].Provider != "project-agent" {
		t.Fatalf("project capabilities = %#v", projectItems)
	}
	if host.calls != 1 || project.calls != 1 {
		t.Fatalf("scope-filtered calls = host:%d project:%d", host.calls, project.calls)
	}
}

func TestListUsesModuleIdentityAndPublishesDefensiveMetadata(t *testing.T) {
	provider := &catalogTestProvider{id: "future-agent", reportedID: "wrong-agent", label: "Adapter Label"}
	modules := catalogTestModules{"future-agent": {
		ID:               "future-agent",
		Label:            "Future Agent",
		Default:          true,
		ExecutionScopes:  []agentmodule.ExecutionScope{agentmodule.ScopeHost},
		Auth:             agentmodule.AuthExternal,
		AuthInstructions: "Run future-agent login.",
		Features: agentmodule.Features{
			Sessions:       agentmodule.SessionSupport{Resume: true},
			Skills:         agentmodule.SkillsInstructions,
			ScheduledTools: true,
		},
	}}
	catalog := New(
		catalogTestRegistry{providers: []agent.CapabilityProvider{provider}},
		nil,
		nil,
		testSettings(),
		WithModulePolicy(modules),
	)
	items, err := catalog.List(context.Background(), ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("capabilities = %#v", items)
	}
	got := items[0]
	if got.Provider != "future-agent" || got.Label != "Future Agent" || !got.Default ||
		len(got.ExecutionScopes) != 1 || got.ExecutionScopes[0] != "host" ||
		got.Authentication.Mode != "external" || got.Authentication.Instructions != "Run future-agent login." ||
		!got.Features.Sessions.Resume || got.Features.Skills != "instructions" || !got.Features.ScheduledTools {
		t.Fatalf("decorated capabilities = %#v", got)
	}
	items[0].ExecutionScopes[0] = "changed"
	cached, err := catalog.List(context.Background(), ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if cached[0].ExecutionScopes[0] != "host" {
		t.Fatalf("cached metadata mutated through response: %#v", cached[0])
	}
}

func TestListCoalescesOnlyOverlappingRequests(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	provider := &catalogTestProvider{
		id: agent.ProviderClaude, label: "Claude", entered: entered, release: release,
	}
	catalog := New(catalogTestRegistry{providers: []agent.CapabilityProvider{provider}}, nil, nil, testSettings())
	firstDone := make(chan error, 1)
	go func() {
		_, err := catalog.List(context.Background(), ListQuery{})
		firstDone <- err
	}()
	<-entered

	waiterCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := catalog.List(waiterCtx, ListQuery{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("overlapping waiter error = %v", err)
	}
	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls != 1 {
		t.Fatalf("overlapping capability calls = %d", calls)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.List(context.Background(), ListQuery{}); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	calls = provider.calls
	provider.mu.Unlock()
	if calls != 1 {
		t.Fatalf("cached capability calls = %d", calls)
	}
	if _, err := catalog.List(context.Background(), ListQuery{Refresh: true}); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	calls = provider.calls
	provider.mu.Unlock()
	if calls != 2 {
		t.Fatalf("refreshed capability calls = %d", calls)
	}
}

func TestCatalogTTLUsesShortRetryForDegradedResults(t *testing.T) {
	cache := newCatalogCache(24*time.Hour, 2*time.Hour)
	if got := cache.ttl([]agent.Capabilities{{Source: agent.CapabilitySourceLive}}); got != 24*time.Hour {
		t.Fatalf("live TTL = %s", got)
	}
	if got := cache.ttl([]agent.Capabilities{{Source: agent.CapabilitySourceFallback}}); got != 2*time.Hour {
		t.Fatalf("fallback TTL = %s", got)
	}
	if got := cache.ttl([]agent.Capabilities{{Source: agent.CapabilitySourceLive, Warning: "partial"}}); got != 2*time.Hour {
		t.Fatalf("warning TTL = %s", got)
	}
}

func testSettings() Settings {
	return Settings{
		CapabilityTimeout:          30 * time.Second,
		CapabilityCacheTTL:         24 * time.Hour,
		DegradedCapabilityCacheTTL: 2 * time.Hour,
	}
}

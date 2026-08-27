package agent

import (
	"errors"
	"fmt"
)

var ErrInvalidProvider = errors.New("invalid agent provider")

// Registry owns the configured agent implementations, keyed by their stable
// provider identifier. Agents are registered once at the composition root and
// looked up by application services at run time.
type Registry struct {
	providers map[ProviderID]Provider
	order     []ProviderID
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[ProviderID]Provider)}
}

func (r *Registry) Register(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("%w: provider is nil", ErrInvalidProvider)
	}
	id := provider.ID()
	if id == "" {
		return fmt.Errorf("%w: provider ID is empty", ErrInvalidProvider)
	}
	if r.providers == nil {
		r.providers = make(map[ProviderID]Provider)
	}
	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("%w: provider %q is already registered", ErrInvalidProvider, id)
	}
	r.providers[id] = provider
	r.order = append(r.order, id)
	return nil
}

func (r *Registry) Lookup(id ProviderID) Provider {
	if r == nil {
		return nil
	}
	return r.providers[id]
}

// CapabilityProviders returns registered providers in composition order using
// only the capability-discovery contract needed by catalog consumers.
func (r *Registry) CapabilityProviders() []CapabilityProvider {
	if r == nil {
		return nil
	}
	providers := make([]CapabilityProvider, 0, len(r.order))
	for _, id := range r.order {
		if provider := r.providers[id]; provider != nil {
			providers = append(providers, provider)
		}
	}
	return providers
}

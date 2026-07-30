package auth

import (
	"errors"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

var ErrInvalidBinding = errors.New("invalid agent auth binding")

// Registry owns the auth callers configured by the agent catalog. Bindings
// retain catalog order so route registration and diagnostics are deterministic.
type Registry struct {
	bindings []Binding
	byID     map[agent.ProviderID]int
}

func NewRegistry() *Registry {
	return &Registry{byID: make(map[agent.ProviderID]int)}
}

func (r *Registry) Register(binding Binding) error {
	if r == nil {
		return fmt.Errorf("%w: registry is nil", ErrInvalidBinding)
	}
	if binding.ID() == "" {
		return fmt.Errorf("%w: provider ID is empty", ErrInvalidBinding)
	}
	if binding.Flow() != FlowCode && binding.Flow() != FlowDevice && binding.Flow() != FlowAPIKey {
		return fmt.Errorf("%w: provider %q has unknown flow %q", ErrInvalidBinding, binding.ID(), binding.Flow())
	}
	if r.byID == nil {
		r.byID = make(map[agent.ProviderID]int)
	}
	if _, exists := r.byID[binding.ID()]; exists {
		return fmt.Errorf("%w: provider %q is already registered", ErrInvalidBinding, binding.ID())
	}
	r.byID[binding.ID()] = len(r.bindings)
	r.bindings = append(r.bindings, binding)
	return nil
}

func (r *Registry) Lookup(id agent.ProviderID) (Binding, bool) {
	if r == nil {
		return Binding{}, false
	}
	index, ok := r.byID[id]
	if !ok {
		return Binding{}, false
	}
	return r.bindings[index], true
}

func (r *Registry) Bindings() []Binding {
	if r == nil {
		return nil
	}
	return append([]Binding(nil), r.bindings...)
}

// AnyAuthenticated reports whether at least one registered provider has a
// usable host-side login. The application access gate uses this to keep the
// workspace closed until initial provider setup is complete.
func (r *Registry) AnyAuthenticated() bool {
	if r == nil {
		return false
	}
	for _, binding := range r.bindings {
		if binding.Authenticated() {
			return true
		}
	}
	return false
}

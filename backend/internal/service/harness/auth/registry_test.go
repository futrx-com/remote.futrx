package auth

import (
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

func TestRegistryPreservesOrderAndReturnsDefensiveCopies(t *testing.T) {
	registry := NewRegistry()
	claude := NewCodeBinding(harness.ProviderClaude, nil)
	codex := NewDeviceBinding[bindingTestStatus](harness.ProviderCodex, nil)
	for _, binding := range []Binding{claude, codex} {
		if err := registry.Register(binding); err != nil {
			t.Fatalf("Register(%q): %v", binding.ID(), err)
		}
	}

	bindings := registry.Bindings()
	if len(bindings) != 2 || bindings[0].ID() != harness.ProviderClaude || bindings[1].ID() != harness.ProviderCodex {
		t.Fatalf("Bindings = %#v", bindings)
	}
	bindings[0] = Binding{}
	if got := registry.Bindings()[0].ID(); got != harness.ProviderClaude {
		t.Fatalf("registry mutated through returned slice: first ID = %q", got)
	}
	if got, ok := registry.Lookup(harness.ProviderCodex); !ok || got.ID() != harness.ProviderCodex {
		t.Fatalf("Lookup(codex) = (%q, %v)", got.ID(), ok)
	}
}

func TestRegistryRejectsInvalidAndDuplicateBindings(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Binding{}); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("Register(empty) error = %v", err)
	}
	binding := NewCodeBinding(harness.ProviderClaude, nil)
	if err := registry.Register(binding); err != nil {
		t.Fatalf("Register(claude): %v", err)
	}
	if err := registry.Register(binding); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("Register(duplicate) error = %v", err)
	}
}

func TestRegistryRequiresAtLeastOneAuthenticatedProvider(t *testing.T) {
	registry := NewRegistry()
	authenticated := false
	service := NewDeviceService(DeviceConfig[bindingTestStatus]{
		Authenticated: func() bool { return authenticated },
		BuildStatus: func() DeviceStatusBuilder[bindingTestStatus] {
			return func(state DeviceState) bindingTestStatus {
				return bindingTestStatus{Authenticated: authenticated, DeviceLogin: state}
			}
		},
	})
	if err := registry.Register(NewDeviceBinding(harness.ProviderCodex, service)); err != nil {
		t.Fatalf("Register(codex): %v", err)
	}
	if registry.AnyAuthenticated() {
		t.Fatal("registry opened without a provider login")
	}
	authenticated = true
	if !registry.AnyAuthenticated() {
		t.Fatal("registry stayed closed after a provider login")
	}
}

package permission

import (
	"errors"
	"testing"
)

func stateTestRegistry() *Registry {
	return MustRegistry([]Definition{validDefinition("projects.lifecycle.manage")}, ManagementDefinitions())
}

func TestValidateAgainstRejectsUnknownPermissionKeys(t *testing.T) {
	registry := stateTestRegistry()
	tests := []struct {
		name  string
		state State
	}{
		{"assignment", State{Assignments: []Assignment{{
			ID: "a", UserEmail: "a@example.com", Permission: "projects.removed.manage",
			Effect: Deny, Scope: ProjectScope("abcd"),
		}}}},
		{"role rule", State{Roles: []Role{{
			ID: "r", Name: "R", Rules: []RoleRule{{Permission: "projects.removed.manage", Effect: Allow}},
		}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.state.ValidateStructure(); err != nil {
				t.Fatalf("ValidateStructure() = %v, want the state to be structurally valid", err)
			}
			err := test.state.ValidateAgainst(registry)
			if !errors.Is(err, ErrInvalidState) {
				t.Fatalf("ValidateAgainst() error = %v, want ErrInvalidState", err)
			}
		})
	}
}

func TestValidateAgainstRejectsUnsupportedScopeKinds(t *testing.T) {
	registry := stateTestRegistry()
	assignment := State{Assignments: []Assignment{{
		ID: "a", UserEmail: "a@example.com", Permission: "projects.lifecycle.manage",
		Effect: Deny, Scope: PlatformScope(),
	}}}
	binding := State{
		Roles: []Role{{ID: "r", Name: "R", Rules: []RoleRule{{Permission: "projects.lifecycle.manage", Effect: Allow}}}},
		Bindings: []RoleBinding{{
			ID: "b", RoleID: "r", UserEmail: "a@example.com", Scope: PlatformScope(),
		}},
	}
	for name, state := range map[string]State{"assignment": assignment, "role binding": binding} {
		t.Run(name, func(t *testing.T) {
			if err := state.ValidateAgainst(registry); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("ValidateAgainst() error = %v, want ErrInvalidState", err)
			}
		})
	}
}

func TestValidateAgainstAcceptsRegisteredPolicy(t *testing.T) {
	state := State{
		Assignments: []Assignment{{
			ID: "a", UserEmail: "a@example.com", Permission: "projects.lifecycle.manage",
			Effect: Deny, Scope: ProjectScope("abcd"),
		}},
		Roles: []Role{{ID: "r", Name: "R", Rules: []RoleRule{{Permission: "projects.lifecycle.manage", Effect: Allow}}}},
		Bindings: []RoleBinding{{
			ID: "b", RoleID: "r", UserEmail: "a@example.com", Scope: ProjectScope("abcd"),
		}},
	}
	if err := state.ValidateStructure(); err != nil {
		t.Fatal(err)
	}
	if err := state.ValidateAgainst(stateTestRegistry()); err != nil {
		t.Fatal(err)
	}
}

func TestCloneIsDeep(t *testing.T) {
	original := State{
		Assignments: []Assignment{{ID: "a"}},
		Roles:       []Role{{ID: "r", Rules: []RoleRule{{Permission: "x.y.z", Effect: Allow}}}},
		Bindings:    []RoleBinding{{ID: "b"}},
	}
	clone := original.Clone()
	clone.Roles[0].Rules[0].Effect = Deny
	clone.Assignments[0].ID = "changed"
	clone.Bindings[0].ID = "changed"

	if original.Roles[0].Rules[0].Effect != Allow || original.Assignments[0].ID != "a" || original.Bindings[0].ID != "b" {
		t.Fatalf("Clone shares memory with the original: %#v", original)
	}
}

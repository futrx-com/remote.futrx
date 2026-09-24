package permission

import (
	"errors"
	"testing"
)

func validDefinition(key Key) Definition {
	return Definition{
		Key:         key,
		Description: "Test permission.",
		Scopes:      []ScopeKind{ScopeProject},
		Baseline:    BaselineProjectMember,
		Delegable:   true,
	}
}

func TestNewRegistryAcceptsUniqueDefinitionsAcrossGroups(t *testing.T) {
	registry, err := NewRegistry(
		[]Definition{validDefinition("projects.lifecycle.manage")},
		ManagementDefinitions(),
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	keys := make([]Key, 0)
	for _, definition := range registry.Definitions() {
		keys = append(keys, definition.Key)
	}
	want := []Key{PermissionAssignmentsManage, PermissionRolesManage, "projects.lifecycle.manage"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %v, want sorted %v", keys, want)
		}
	}
}

func TestNewRegistryRejectsInvalidDefinitions(t *testing.T) {
	mutate := func(edit func(*Definition)) Definition {
		definition := validDefinition("projects.lifecycle.manage")
		edit(&definition)
		return definition
	}
	tests := []struct {
		name       string
		definition Definition
	}{
		{"empty key", mutate(func(d *Definition) { d.Key = "" })},
		{"two segments", mutate(func(d *Definition) { d.Key = "projects.manage" })},
		{"four segments", mutate(func(d *Definition) { d.Key = "projects.a.b.c" })},
		{"uppercase", mutate(func(d *Definition) { d.Key = "Projects.lifecycle.manage" })},
		{"whitespace", mutate(func(d *Definition) { d.Key = "projects. lifecycle.manage" })},
		{"leading digit", mutate(func(d *Definition) { d.Key = "1projects.lifecycle.manage" })},
		{"empty segment", mutate(func(d *Definition) { d.Key = "projects..manage" })},
		{"empty description", mutate(func(d *Definition) { d.Description = "  " })},
		{"no scopes", mutate(func(d *Definition) { d.Scopes = nil })},
		{"unknown scope", mutate(func(d *Definition) { d.Scopes = []ScopeKind{"team"} })},
		{"repeated scope", mutate(func(d *Definition) { d.Scopes = []ScopeKind{ScopeProject, ScopeProject} })},
		{"unknown baseline", mutate(func(d *Definition) { d.Baseline = "everyone" })},
		{"member baseline without project scope", mutate(func(d *Definition) {
			d.Scopes = []ScopeKind{ScopePlatform}
		})},
		{"member baseline with extra scope", mutate(func(d *Definition) {
			d.Scopes = []ScopeKind{ScopeProject, ScopePlatform}
		})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewRegistry([]Definition{test.definition})
			if !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("NewRegistry() error = %v, want ErrInvalidDefinition", err)
			}
		})
	}
}

func TestNewRegistryRejectsDuplicateKeysWithinAndAcrossGroups(t *testing.T) {
	definition := validDefinition("projects.lifecycle.manage")
	for name, groups := range map[string][][]Definition{
		"within a group":   {{definition, definition}},
		"across groups":    {{definition}, {definition}},
		"empty catalog":    {},
		"only empty group": {nil},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRegistry(groups...); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("NewRegistry() error = %v, want ErrInvalidDefinition", err)
			}
		})
	}
}

func TestMustRegistryPanicsOnInvalidCatalog(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustRegistry() did not panic on a malformed definition")
		}
	}()
	MustRegistry([]Definition{{Key: "bad"}})
}

func TestUndeclaredBaselineDefaultsToNone(t *testing.T) {
	definition := validDefinition("projects.lifecycle.manage")
	definition.Baseline = ""
	definition.Scopes = []ScopeKind{ScopePlatform}
	registry, err := NewRegistry([]Definition{definition})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := registry.Lookup(definition.Key)
	if got.Baseline != BaselineNone {
		t.Fatalf("baseline = %q, want %q", got.Baseline, BaselineNone)
	}
}

func TestRegistryIsImmutableThroughReturnedDefinitions(t *testing.T) {
	registry := MustRegistry([]Definition{validDefinition("projects.lifecycle.manage")})

	got, _ := registry.Lookup("projects.lifecycle.manage")
	got.Scopes[0] = ScopePlatform
	got.Description = "changed"
	listed := registry.Definitions()
	listed[0].Scopes[0] = ScopePlatform

	again, _ := registry.Lookup("projects.lifecycle.manage")
	if again.Scopes[0] != ScopeProject || again.Description != "Test permission." {
		t.Fatalf("registry mutated through a returned definition: %#v", again)
	}
}

func TestRegistryResolveValidatesChecks(t *testing.T) {
	registry := MustRegistry([]Definition{validDefinition("projects.lifecycle.manage")}, ManagementDefinitions())
	tests := []struct {
		name  string
		check Check
		want  error
	}{
		{"valid", Check{"projects.lifecycle.manage", ProjectScope("abcd")}, nil},
		{"unregistered key", Check{"projects.other.manage", ProjectScope("abcd")}, ErrUnknownPermission},
		{"unsupported scope kind", Check{"projects.lifecycle.manage", PlatformScope()}, ErrInvalidScope},
		{"malformed project id", Check{"projects.lifecycle.manage", ProjectScope("../x")}, ErrInvalidScope},
		{"project scope without id", Check{"projects.lifecycle.manage", ProjectScope("")}, ErrInvalidScope},
		{"platform scope with id", Check{PermissionRolesManage, Scope{Kind: ScopePlatform, ID: "x"}}, ErrInvalidScope},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := registry.resolve(test.check)
			if !errors.Is(err, test.want) && !(test.want == nil && err == nil) {
				t.Fatalf("resolve() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestManagementPermissionsAreAdministratorOnlyAndNotDelegable(t *testing.T) {
	for _, definition := range ManagementDefinitions() {
		if definition.Delegable {
			t.Fatalf("%s must not be delegable", definition.Key)
		}
		if definition.Baseline != BaselineNone {
			t.Fatalf("%s baseline = %q, want none", definition.Key, definition.Baseline)
		}
		if len(definition.Scopes) != 1 || definition.Scopes[0] != ScopePlatform {
			t.Fatalf("%s scopes = %v, want platform only", definition.Key, definition.Scopes)
		}
	}
}

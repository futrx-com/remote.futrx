package project

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/permission"
)

// allowAllAuthorizer lets tests that exercise lifecycle behavior, not
// authorization, call the protected entry points directly.
type allowAllAuthorizer struct{}

func (allowAllAuthorizer) Require(context.Context, permission.Check) error { return nil }

// recordingAuthorizer records every check and answers with a fixed error.
type recordingAuthorizer struct {
	mu     sync.Mutex
	checks []permission.Check
	err    error
}

func (a *recordingAuthorizer) Require(_ context.Context, check permission.Check) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.checks = append(a.checks, check)
	return a.err
}

func (a *recordingAuthorizer) recorded() []permission.Check {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]permission.Check(nil), a.checks...)
}

// accessTestRepository is an in-memory AccessRepository.
type accessTestRepository struct {
	mu      sync.Mutex
	members map[ID]map[string]bool
}

func (r *accessTestRepository) List(_ context.Context, id ID) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for email := range r.members[id] {
		out = append(out, email)
	}
	sort.Strings(out)
	return out, nil
}

func (r *accessTestRepository) Add(_ context.Context, id ID, email string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.members == nil {
		r.members = map[ID]map[string]bool{}
	}
	if r.members[id] == nil {
		r.members[id] = map[string]bool{}
	}
	r.members[id][email] = true
	return nil
}

func (r *accessTestRepository) Remove(_ context.Context, id ID, email string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.members[id], email)
	return nil
}

func (r *accessTestRepository) Set(context.Context, ID, []string) error { return nil }

func (r *accessTestRepository) Has(_ context.Context, id ID, email string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.members[id][email], nil
}

func (r *accessTestRepository) DeleteAll(context.Context, ID) error { return nil }

// missingProjectRepository reports every project as absent.
type missingProjectRepository struct{ *startTestRepository }

func (missingProjectRepository) Get(context.Context, ID) (Meta, error) {
	return Meta{}, ErrNotFound
}

func lifecycleCalls(l *startTestLifecycle) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.launchCalls + l.startCalls + l.restartCalls
}

type protectedCall struct {
	name string
	key  permission.Key
	call func(context.Context, *Service, ID) error
}

func protectedCalls() []protectedCall {
	return []protectedCall{
		{"Start", PermissionLifecycleManage, func(ctx context.Context, s *Service, id ID) error {
			_, err := s.Start(ctx, id)
			return err
		}},
		{"Stop", PermissionLifecycleManage, func(ctx context.Context, s *Service, id ID) error {
			_, err := s.Stop(ctx, id)
			return err
		}},
		{"Restart", PermissionLifecycleManage, func(ctx context.Context, s *Service, id ID) error {
			_, err := s.Restart(ctx, id)
			return err
		}},
		{"RepairNetwork", PermissionLifecycleManage, func(ctx context.Context, s *Service, id ID) error {
			_, err := s.RepairNetwork(ctx, id)
			return err
		}},
		{"ListAccess", PermissionAccessManage, func(ctx context.Context, s *Service, id ID) error {
			_, err := s.ListAccess(ctx, id)
			return err
		}},
		{"AddAccess", PermissionAccessManage, func(ctx context.Context, s *Service, id ID) error {
			return s.AddAccess(ctx, id, "member@example.com")
		}},
		{"RemoveAccess", PermissionAccessManage, func(ctx context.Context, s *Service, id ID) error {
			return s.RemoveAccess(ctx, id, "member@example.com")
		}},
	}
}

func authorizationTestService(authorizer Authorizer) (*Service, *startTestRepository, *startTestLifecycle) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "project", ContainerName: "project", Status: StatusRunning,
	}}
	lifecycle := &startTestLifecycle{state: ContainerStateRunning}
	var options []Option
	if authorizer != nil {
		options = append(options, WithAuthorizer(authorizer))
	}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle}, nil, &accessTestRepository{}, options...)
	return service, repo, lifecycle
}

func TestProtectedEntryPointsRequireTheirPermissionAtProjectScope(t *testing.T) {
	for _, test := range protectedCalls() {
		t.Run(test.name, func(t *testing.T) {
			authorizer := &recordingAuthorizer{}
			service, _, _ := authorizationTestService(authorizer)

			if err := test.call(context.Background(), service, "abcd"); err != nil {
				t.Fatalf("%s() error = %v", test.name, err)
			}
			checks := authorizer.recorded()
			want := permission.Check{Permission: test.key, Scope: permission.ProjectScope("abcd")}
			if len(checks) != 1 || checks[0] != want {
				t.Fatalf("checks = %#v, want exactly %#v", checks, want)
			}
		})
	}
}

func TestDeniedCallsStopBeforeAnySideEffect(t *testing.T) {
	for _, test := range protectedCalls() {
		t.Run(test.name, func(t *testing.T) {
			service, repo, lifecycle := authorizationTestService(&recordingAuthorizer{err: permission.ErrDenied})

			err := test.call(context.Background(), service, "abcd")
			if !errors.Is(err, permission.ErrDenied) {
				t.Fatalf("%s() error = %v, want ErrDenied", test.name, err)
			}
			if repo.meta.Status != StatusRunning {
				t.Fatalf("status = %q, want the project untouched", repo.meta.Status)
			}
			if calls := lifecycleCalls(lifecycle); calls != 0 {
				t.Fatalf("container lifecycle was called %d times after a denial", calls)
			}
			members, _ := service.access.list(context.Background(), "abcd")
			if len(members) != 0 {
				t.Fatalf("members = %v, want the access list untouched", members)
			}
		})
	}
}

func TestAuthorizationPrecedesDomainErrors(t *testing.T) {
	// A caller who may not manage a project must not learn whether it exists.
	service := New(
		missingProjectRepository{&startTestRepository{}},
		ContainerDependencies{}, nil, &accessTestRepository{},
		WithAuthorizer(&recordingAuthorizer{err: permission.ErrDenied}),
	)

	for _, test := range protectedCalls() {
		err := test.call(context.Background(), service, "beef")
		if !errors.Is(err, permission.ErrDenied) {
			t.Fatalf("%s() on a missing project error = %v, want ErrDenied", test.name, err)
		}
	}
}

func TestMalformedIdentifiersKeepTheirErrorAndSkipAuthorization(t *testing.T) {
	for _, test := range protectedCalls() {
		t.Run(test.name, func(t *testing.T) {
			authorizer := &recordingAuthorizer{}
			service, _, _ := authorizationTestService(authorizer)

			if err := test.call(context.Background(), service, "../x"); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("%s() error = %v, want ErrInvalidID", test.name, err)
			}
			if len(authorizer.recorded()) != 0 {
				t.Fatal("a malformed id reached the authorizer")
			}
		})
	}
}

func TestServiceWithoutAnAuthorizerFailsClosed(t *testing.T) {
	system := permission.ContextWithSystemActor(context.Background())
	user := permission.ContextWithActor(context.Background(), permission.UserActor("member@example.com"))

	for _, test := range protectedCalls() {
		t.Run(test.name, func(t *testing.T) {
			service, _, _ := authorizationTestService(nil)

			if err := test.call(context.Background(), service, "abcd"); !errors.Is(err, permission.ErrActorRequired) {
				t.Fatalf("no actor: error = %v, want ErrActorRequired", err)
			}
			if err := test.call(user, service, "abcd"); !errors.Is(err, permission.ErrDenied) {
				t.Fatalf("human actor: error = %v, want ErrDenied", err)
			}
			if err := test.call(system, service, "abcd"); err != nil {
				t.Fatalf("system actor: error = %v, want nil", err)
			}
		})
	}
}

func TestAccessManagementKeepsItsDomainInvariantsAfterAuthorization(t *testing.T) {
	service, _, _ := authorizationTestService(allowAllAuthorizer{})

	if err := service.AddAccess(context.Background(), "abcd", "  "); err == nil ||
		!strings.Contains(err.Error(), "empty email") {
		t.Fatalf("AddAccess(empty email) error = %v, want the domain validation error", err)
	}
	if err := service.AddAccess(context.Background(), "abcd", "Member@Example.com"); err != nil {
		t.Fatal(err)
	}
	members, err := service.ListAccess(context.Background(), "abcd")
	if err != nil || len(members) != 1 || members[0] != "member@example.com" {
		t.Fatalf("ListAccess() = %v, %v; want the normalized member", members, err)
	}
}

// StartAgentBrowser acts under its own capability; it must not start
// consulting the lifecycle permission through Start.
func TestAgentBrowserStartIsNotGatedByTheLifecyclePermission(t *testing.T) {
	authorizer := &recordingAuthorizer{err: permission.ErrDenied}
	repo := &startTestRepository{meta: Meta{ID: "abcd", Name: "p", ContainerName: "p", Status: StatusStopped}}
	lifecycle := &startTestLifecycle{state: ContainerStateRunning}
	service := New(
		repo, ContainerDependencies{Lifecycle: lifecycle, Browser: &upgradeTestBrowser{}}, nil, nil,
		WithAuthorizer(authorizer),
	)

	_, _ = service.StartAgentBrowser(context.Background(), "abcd")

	if checks := authorizer.recorded(); len(checks) != 0 {
		t.Fatalf("StartAgentBrowser consulted the authorizer: %#v", checks)
	}
}

func TestPermissionDefinitionsRegisterCleanly(t *testing.T) {
	registry, err := permission.NewRegistry(PermissionDefinitions())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	for _, key := range []permission.Key{PermissionLifecycleManage, PermissionAccessManage} {
		definition, ok := registry.Lookup(key)
		if !ok {
			t.Fatalf("%s was not registered", key)
		}
		if definition.Baseline != permission.BaselineProjectMember || !definition.Delegable ||
			len(definition.Scopes) != 1 || definition.Scopes[0] != permission.ScopeProject {
			t.Fatalf("%s = %#v, want a delegable project-scoped member-baseline permission", key, definition)
		}
	}
}

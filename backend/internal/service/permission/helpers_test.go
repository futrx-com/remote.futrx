package permission

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

const (
	testAdmin    = "admin@example.com"
	testManager  = "manager@example.com"
	testAlice    = "alice@example.com"
	testBob      = "bob@example.com"
	testOutsider = "outsider@example.com"
	testUnknown  = "unknown@example.com"

	projectA = "aaaa1111"
	projectB = "bbbb2222"

	permLifecycle = Key("projects.lifecycle.manage")
	permAccess    = Key("projects.access.manage")
	permSecrets   = Key("projects.secrets.reveal") // project scope, no baseline, not delegable
	permDocs      = Key("docs.pages.edit")         // platform and project scope, delegable
	permBilling   = Key("billing.data.export")     // platform scope, not delegable
	permReports   = Key("reports.summary.view")    // platform scope, authenticated baseline
	permAdminOnly = Key("system.tools.run")        // platform scope, admin baseline
)

func testRegistry() *Registry {
	return MustRegistry(
		[]Definition{
			{Key: permLifecycle, Description: "Start and stop projects.", Scopes: []ScopeKind{ScopeProject},
				Baseline: BaselineProjectMember, Delegable: true},
			{Key: permAccess, Description: "Manage project members.", Scopes: []ScopeKind{ScopeProject},
				Baseline: BaselineProjectMember, Delegable: true},
			{Key: permSecrets, Description: "Reveal project secrets.", Scopes: []ScopeKind{ScopeProject}},
			{Key: permDocs, Description: "Edit pages.", Scopes: []ScopeKind{ScopePlatform, ScopeProject},
				Delegable: true},
			{Key: permBilling, Description: "Export billing data.", Scopes: []ScopeKind{ScopePlatform}},
			{Key: permReports, Description: "View summaries.", Scopes: []ScopeKind{ScopePlatform},
				Baseline: BaselineAuthenticated, Delegable: true},
			{Key: permAdminOnly, Description: "Run system tools.", Scopes: []ScopeKind{ScopePlatform},
				Baseline: BaselineAdmin},
		},
		ManagementDefinitions(),
	)
}

// memoryRepository is an in-memory Repository that keeps a chronological audit
// log and can be told to fail audit writes.
type memoryRepository struct {
	mu        sync.Mutex
	state     State
	audit     []AuditEvent
	failAudit error
}

func (r *memoryRepository) Load(context.Context) (State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.Clone(), nil
}

func (r *memoryRepository) Mutate(
	_ context.Context,
	change func(State) (State, []AuditEvent, error),
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	next, events, err := change(r.state.Clone())
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	if err := next.ValidateStructure(); err != nil {
		return err
	}
	if r.failAudit != nil {
		return fmt.Errorf("%w: %v", ErrAuditFailed, r.failAudit)
	}
	r.audit = append(r.audit, events...)
	r.state = next.Clone()
	return nil
}

func (r *memoryRepository) auditLog() []AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]AuditEvent(nil), r.audit...)
}

type fakeIdentity struct {
	admins     map[string]bool
	registered map[string]bool
	err        error
}

func (f *fakeIdentity) IsAdmin(_ context.Context, email string) (bool, error) {
	return f.admins[email], f.err
}

func (f *fakeIdentity) IsRegistered(_ context.Context, email string) (bool, error) {
	return f.registered[email] || f.admins[email], f.err
}

type fakeMembers map[string]map[string]bool // project -> email -> member

func (m fakeMembers) HasAccess(_ context.Context, projectID, email string) (bool, error) {
	return m[projectID][email], nil
}

type fixture struct {
	service  *Service
	repo     *memoryRepository
	identity *fakeIdentity
	members  fakeMembers
	clock    time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		repo: &memoryRepository{},
		identity: &fakeIdentity{
			admins:     map[string]bool{testAdmin: true},
			registered: map[string]bool{testManager: true, testAlice: true, testBob: true, testOutsider: true},
		},
		members: fakeMembers{
			projectA: {testManager: true, testAlice: true},
		},
		clock: time.Unix(1_700_000_000, 0),
	}
	next := 0
	service, err := NewService(
		context.Background(), testRegistry(), f.repo, f.identity, f.members,
		WithClock(func() time.Time { f.clock = f.clock.Add(time.Second); return f.clock }),
		WithIDGenerator(func() (string, error) { next++; return fmt.Sprintf("id%03d", next), nil }),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	f.service = service
	return f
}

func as(email string) context.Context {
	return ContextWithActor(context.Background(), UserActor(email))
}

func mustSet(t *testing.T, service *Service, ctx context.Context, email string, key Key, effect Effect, scope Scope) {
	t.Helper()
	if _, err := service.SetAssignment(ctx, AssignmentInput{
		UserEmail: email, Permission: key, Effect: effect, Scope: scope,
	}); err != nil {
		t.Fatalf("SetAssignment(%s %s %s %s) error = %v", email, key, effect, scope, err)
	}
}

func mustRole(t *testing.T, service *Service, name string, rules ...RoleRule) Role {
	t.Helper()
	role, err := service.CreateRole(as(testAdmin), RoleInput{Name: name, Rules: rules})
	if err != nil {
		t.Fatalf("CreateRole(%s) error = %v", name, err)
	}
	return role
}

func mustBind(t *testing.T, service *Service, ctx context.Context, roleID, email string, scope Scope) {
	t.Helper()
	if _, err := service.BindRole(ctx, BindingInput{RoleID: roleID, UserEmail: email, Scope: scope}); err != nil {
		t.Fatalf("BindRole(%s, %s, %s) error = %v", roleID, email, scope, err)
	}
}

func requireAllowed(t *testing.T, service *Service, ctx context.Context, key Key, scope Scope, want bool) {
	t.Helper()
	got, err := service.Can(ctx, Check{Permission: key, Scope: scope})
	if err != nil {
		t.Fatalf("Can(%s, %s) error = %v", key, scope, err)
	}
	if got != want {
		t.Fatalf("Can(%s, %s) = %v, want %v", key, scope, got, want)
	}
}

func requireError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

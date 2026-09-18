package service

import (
	"context"
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicepermission "github.com/futrx-com/remote.futrx.com/internal/service/permission"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepermissions"
)

const systemCallerProjectID = serviceproject.ID("beefcafe")

// systemCallerRepository is a one-project repository that records status
// transitions, so a test can see whether Start ran.
type systemCallerRepository struct {
	serviceproject.Repository
	status serviceproject.Status
}

func (r *systemCallerRepository) Get(_ context.Context, id serviceproject.ID) (serviceproject.Meta, error) {
	if id != systemCallerProjectID {
		return serviceproject.Meta{}, serviceproject.ErrNotFound
	}
	return serviceproject.Meta{ID: id, Slug: "alpha", Status: r.status}, nil
}

func (r *systemCallerRepository) SetStatus(
	_ context.Context, id serviceproject.ID, status serviceproject.Status, _ string,
) (serviceproject.Meta, error) {
	r.status = status
	return serviceproject.Meta{ID: id, Slug: "alpha", Status: status}, nil
}

// realAuthorizerProjects builds the project service with the production
// registry and a real permission service, and denies "member@example.com"
// lifecycle and access management with explicit assignments.
func realAuthorizerProjects(t *testing.T) (*serviceproject.Service, *systemCallerRepository) {
	t.Helper()
	store, err := filepermissions.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	access := &cleanupProjectAccess{members: map[serviceproject.ID][]string{
		systemCallerProjectID: {"member@example.com"},
	}}
	permissions, err := newPermissions(context.Background(), store, newTestPermissionIdentity(), access)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []servicepermission.Key{
		serviceproject.PermissionLifecycleManage, serviceproject.PermissionAccessManage,
	} {
		if _, err := permissions.SetAssignment(actorContext("admin@example.com"), servicepermission.AssignmentInput{
			UserEmail: "member@example.com", Permission: key, Effect: servicepermission.Deny,
			Scope: servicepermission.ProjectScope(string(systemCallerProjectID)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	repo := &systemCallerRepository{status: serviceproject.StatusStopped}
	projects := serviceproject.New(
		repo, serviceproject.ContainerDependencies{}, nil, access,
		serviceproject.WithAuthorizer(permissions),
	)
	return projects, repo
}

func TestPermissionCatalogRegistersEveryProjectPermission(t *testing.T) {
	registry, err := servicepermission.NewRegistry(permissionDefinitions()...)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []servicepermission.Key{
		serviceproject.PermissionLifecycleManage, serviceproject.PermissionAccessManage,
		servicepermission.PermissionAssignmentsManage, servicepermission.PermissionRolesManage,
	} {
		if _, ok := registry.Lookup(key); !ok {
			t.Errorf("%s is not registered by the composition root", key)
		}
	}
}

// Trusted internal callers must keep working from contexts that carry no
// authenticated actor, even for a user who is explicitly denied the
// permission: their work is not the user's explicit lifecycle action.
func TestTrustedInternalCallersStartProjectsWithoutAnActor(t *testing.T) {
	t.Run("agent runs", func(t *testing.T) {
		projects, repo := realAuthorizerProjects(t)
		project, err := agentProjectResolver{projects: projects}.Start(context.Background(), agent.ProjectID(systemCallerProjectID))
		if err != nil || project.Status != agent.ProjectStatus(serviceproject.StatusRunning) {
			t.Fatalf("Start() = %+v, %v; want a running project", project, err)
		}
		if repo.status != serviceproject.StatusRunning {
			t.Fatalf("status = %q, want running", repo.status)
		}
	})
	t.Run("installed applications", func(t *testing.T) {
		projects, repo := realAuthorizerProjects(t)
		if err := (projectContainersAdapter{projects: projects}).EnsureRunning(context.Background(), string(systemCallerProjectID)); err != nil {
			t.Fatalf("EnsureRunning() error = %v", err)
		}
		if repo.status != serviceproject.StatusRunning {
			t.Fatalf("status = %q, want running", repo.status)
		}
	})
}

func TestTheSameProjectRefusesAnActorlessOrDeniedDirectCaller(t *testing.T) {
	projects, repo := realAuthorizerProjects(t)

	if _, err := projects.Start(context.Background(), systemCallerProjectID); !errors.Is(err, servicepermission.ErrActorRequired) {
		t.Fatalf("Start() without an actor error = %v, want ErrActorRequired", err)
	}
	if _, err := projects.Start(actorContext("member@example.com"), systemCallerProjectID); !errors.Is(err, servicepermission.ErrDenied) {
		t.Fatalf("Start() by the denied member error = %v, want ErrDenied", err)
	}
	if repo.status != serviceproject.StatusStopped {
		t.Fatalf("status = %q, want the project untouched", repo.status)
	}
}

func TestNotificationAudienceReadsMembersWithoutAnActor(t *testing.T) {
	projects, _ := realAuthorizerProjects(t)
	audience := chatNotificationAudience{
		projects: projects,
		users:    registeredUsers{{Email: "member@example.com", Role: serviceuser.RoleMember}},
	}

	recipients, err := audience.recipients(context.Background(), servicechat.Meta{ProjectID: servicechat.ProjectID(systemCallerProjectID)})
	if err != nil {
		t.Fatalf("recipients() error = %v", err)
	}
	if len(recipients) != 1 || recipients[0] != "member@example.com" {
		t.Fatalf("recipients = %v, want the member even though they are denied access management", recipients)
	}
}

type registeredUsers []serviceuser.User

func (u registeredUsers) List(context.Context) ([]serviceuser.User, error) { return u, nil }

// Removing a user must revoke their project access even when the removing
// context carries no actor or an actor without access-management rights.
func TestUserRemovalRevokesProjectAccessRegardlessOfTheCallersPermissions(t *testing.T) {
	access := &cleanupProjectAccess{members: map[serviceproject.ID][]string{
		systemCallerProjectID: {"member@example.com", "other@example.com"},
	}}
	repo := cleanupProjectRepository{projects: []serviceproject.Meta{{ID: systemCallerProjectID}}}
	projects := serviceproject.New(
		repo, serviceproject.ContainerDependencies{}, nil, access,
		serviceproject.WithAuthorizer(denyEverythingAuthorizer{}),
	)

	for name, ctx := range map[string]context.Context{
		"no actor":            context.Background(),
		"a denied human":      actorContext("member@example.com"),
		"the removing member": actorContext("other@example.com"),
	} {
		t.Run(name, func(t *testing.T) {
			access.members[systemCallerProjectID] = []string{"member@example.com", "other@example.com"}
			if err := (userRemovalCleanup{projects: projects}).CleanupRemovedUser(ctx, "member@example.com"); err != nil {
				t.Fatalf("CleanupRemovedUser() error = %v", err)
			}
			if got := access.members[systemCallerProjectID]; len(got) != 1 || got[0] != "other@example.com" {
				t.Fatalf("members = %v, want only other@example.com", got)
			}
		})
	}
}

// denyEverythingAuthorizer denies every human caller and admits only the
// explicit system actor, like an evaluator with no matching policy.
type denyEverythingAuthorizer struct{}

func (denyEverythingAuthorizer) Require(ctx context.Context, _ servicepermission.Check) error {
	if actor, ok := servicepermission.ActorFromContext(ctx); ok && actor.IsSystem() {
		return nil
	}
	return servicepermission.ErrDenied
}

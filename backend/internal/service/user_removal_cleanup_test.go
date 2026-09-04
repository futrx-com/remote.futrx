package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
)

type cleanupProjectRepository struct {
	serviceproject.Repository
	projects []serviceproject.Meta
}

func (r cleanupProjectRepository) List(context.Context) ([]serviceproject.Meta, error) {
	return append([]serviceproject.Meta(nil), r.projects...), nil
}

func (r cleanupProjectRepository) Get(_ context.Context, id serviceproject.ID) (serviceproject.Meta, error) {
	for _, project := range r.projects {
		if project.ID == id {
			return project, nil
		}
	}
	return serviceproject.Meta{}, serviceproject.ErrNotFound
}

type cleanupProjectAccess struct {
	serviceproject.AccessRepository
	members map[serviceproject.ID][]string
}

func (a *cleanupProjectAccess) List(_ context.Context, projectID serviceproject.ID) ([]string, error) {
	return append([]string(nil), a.members[projectID]...), nil
}

func (a *cleanupProjectAccess) Remove(_ context.Context, projectID serviceproject.ID, email string) error {
	kept := a.members[projectID][:0]
	for _, member := range a.members[projectID] {
		if member != email {
			kept = append(kept, member)
		}
	}
	a.members[projectID] = kept
	return nil
}

func TestUserRemovalCleanupRevokesProjectAccessAndPushDevices(t *testing.T) {
	projects := []serviceproject.Meta{{ID: "beefcafe"}, {ID: "deadbeef"}}
	access := &cleanupProjectAccess{members: map[serviceproject.ID][]string{
		"beefcafe": {"member@example.com", "other@example.com"},
		"deadbeef": {"member@example.com"},
	}}
	projectService := serviceproject.New(
		cleanupProjectRepository{projects: projects},
		serviceproject.ContainerDependencies{},
		nil,
		access,
	)
	pushRepo := &pushRepoStub{rows: map[string][]servicepush.Subscription{
		"member@example.com": {{Endpoint: "https://push.example.com/device"}},
	}}
	cleanup := userRemovalCleanup{
		projects:      projectService,
		subscriptions: pushRepo,
	}

	if err := cleanup.CleanupRemovedUser(context.Background(), "member@example.com"); err != nil {
		t.Fatal(err)
	}
	for projectID, members := range access.members {
		for _, member := range members {
			if member == "member@example.com" {
				t.Fatalf("member still has access to project %s", projectID)
			}
		}
	}
	if subscriptions, _ := pushRepo.List(context.Background(), "member@example.com"); len(subscriptions) != 0 {
		t.Fatalf("push subscriptions survived user removal: %+v", subscriptions)
	}
}

// cleanupSecurityStateStub stands in for either per-account security store
// (two-factor enrollment or session registry): both are keyed by email and
// deleted wholesale, so one stub covers both roles.
type cleanupSecurityStateStub struct {
	emails map[string]bool
	err    error
}

func (s *cleanupSecurityStateStub) Delete(_ context.Context, email string) error {
	if s.err != nil {
		return s.err
	}
	delete(s.emails, email)
	return nil
}

func TestUserRemovalCleanupPurgesTwoFactorAndSessionRegistry(t *testing.T) {
	twoFactor := &cleanupSecurityStateStub{emails: map[string]bool{
		"member@example.com": true,
		"other@example.com":  true,
	}}
	sessionRegistry := &cleanupSecurityStateStub{emails: map[string]bool{
		"member@example.com": true,
		"other@example.com":  true,
	}}
	cleanup := userRemovalCleanup{twoFactor: twoFactor, sessionRegistry: sessionRegistry}

	if err := cleanup.CleanupRemovedUser(context.Background(), "member@example.com"); err != nil {
		t.Fatal(err)
	}
	if twoFactor.emails["member@example.com"] {
		t.Fatal("TOTP secret and recovery codes survived user removal")
	}
	if sessionRegistry.emails["member@example.com"] {
		t.Fatal("session registry entry, sign-in history and alert survived user removal")
	}
	if !twoFactor.emails["other@example.com"] || !sessionRegistry.emails["other@example.com"] {
		t.Fatal("cleanup removed security state belonging to another account")
	}
}

// A failing security store must not stop the remaining cleanup, and its error
// must still surface — the same all-or-report contract the other collaborators
// follow so a retry can finish the job.
func TestUserRemovalCleanupReportsSecurityStateFailuresWithoutSkippingTheRest(t *testing.T) {
	twoFactor := &cleanupSecurityStateStub{err: errors.New("disk full")}
	sessionRegistry := &cleanupSecurityStateStub{emails: map[string]bool{"member@example.com": true}}
	cleanup := userRemovalCleanup{twoFactor: twoFactor, sessionRegistry: sessionRegistry}

	err := cleanup.CleanupRemovedUser(context.Background(), "member@example.com")
	if err == nil || !strings.Contains(err.Error(), "remove two-factor enrollment") {
		t.Fatalf("expected the two-factor failure to surface, got %v", err)
	}
	if sessionRegistry.emails["member@example.com"] {
		t.Fatal("session registry cleanup was skipped after the two-factor store failed")
	}
}

// Accounts that never enrolled leave both stores unwired in some compositions;
// cleanup must stay a no-op rather than panicking.
func TestUserRemovalCleanupToleratesUnwiredSecurityStores(t *testing.T) {
	if err := (userRemovalCleanup{}).CleanupRemovedUser(context.Background(), "member@example.com"); err != nil {
		t.Fatal(err)
	}
}

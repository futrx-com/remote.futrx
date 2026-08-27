package service

import (
	"context"
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

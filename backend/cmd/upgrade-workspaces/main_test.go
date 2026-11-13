package main

import (
	"context"
	"testing"

	servicelifecycle "github.com/futrx-com/remote.futrx.com/internal/service/container/lifecycle"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type missingRuntime struct{ servicelifecycle.Runtime }

func (missingRuntime) State(context.Context, string) (serviceproject.ContainerState, error) {
	return serviceproject.ContainerStateMissing, nil
}

type oneProjectRepository struct {
	serviceproject.Repository
	meta serviceproject.Meta
}

func (r oneProjectRepository) List(context.Context) ([]serviceproject.Meta, error) {
	return []serviceproject.Meta{r.meta}, nil
}

func (r oneProjectRepository) Get(context.Context, serviceproject.ID) (serviceproject.Meta, error) {
	return r.meta, nil
}

// The command runs as root with no authenticated user, while Upgrade is
// guarded by projects.controls.manage. Without the system actor every
// workspace fails with "authenticated actor required".
func TestUpgradeAllActsAsTheSystemActor(t *testing.T) {
	repo := oneProjectRepository{meta: serviceproject.Meta{ID: "abcd", Slug: "alpha", ContainerName: "alpha"}}
	projects := serviceproject.New(repo, serviceproject.ContainerDependencies{}, nil, nil)
	lifecycle := servicelifecycle.NewService(missingRuntime{}, "", nil, nil, nil, nil)
	restored := 0

	upgraded, skipped, failed, err := upgradeAll(
		context.Background(), projects, lifecycle, false, false, newProgressWriter(""),
		func(context.Context, string) error { restored++; return nil },
	)
	if err != nil || upgraded != 1 || skipped != 0 || failed != 0 || restored != 1 {
		t.Fatalf("upgradeAll = %d upgraded, %d skipped, %d failed, %d restored, %v", upgraded, skipped, failed, restored, err)
	}
}

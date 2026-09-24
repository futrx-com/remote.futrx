package service

import (
	"context"
	"errors"
	"testing"

	serviceapplications "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileapplications"
)

type emptyApplicationRegistry struct{}

func (emptyApplicationRegistry) List() []serviceapplications.Application { return nil }
func (emptyApplicationRegistry) Get(string) (serviceapplications.Application, bool) {
	return serviceapplications.Application{}, false
}
func (emptyApplicationRegistry) UIAsset(string, string) ([]byte, bool) { return nil, false }

type failingProjectListRepository struct {
	err   error
	calls int
}

func (r *failingProjectListRepository) List(context.Context) ([]serviceproject.Meta, error) {
	r.calls++
	return nil, r.err
}
func (*failingProjectListRepository) Create(context.Context, serviceproject.Meta) (serviceproject.Meta, error) {
	return serviceproject.Meta{}, nil
}
func (*failingProjectListRepository) Get(context.Context, serviceproject.ID) (serviceproject.Meta, error) {
	return serviceproject.Meta{}, nil
}
func (*failingProjectListRepository) GetBySlug(context.Context, string) (serviceproject.Meta, error) {
	return serviceproject.Meta{}, nil
}
func (*failingProjectListRepository) Update(context.Context, serviceproject.ID, func(*serviceproject.Meta)) (serviceproject.Meta, error) {
	return serviceproject.Meta{}, nil
}
func (*failingProjectListRepository) SetStatus(context.Context, serviceproject.ID, serviceproject.Status, string) (serviceproject.Meta, error) {
	return serviceproject.Meta{}, nil
}
func (*failingProjectListRepository) Delete(context.Context, serviceproject.ID) error { return nil }

type availableProjectLifecycle struct{}

func (availableProjectLifecycle) Available() bool                                   { return true }
func (availableProjectLifecycle) Ensure(context.Context, serviceproject.Meta) error { return nil }
func (availableProjectLifecycle) Busy(context.Context, string) (bool, error)        { return false, nil }
func (availableProjectLifecycle) Start(context.Context, string) error               { return nil }
func (availableProjectLifecycle) Stop(context.Context, string) error                { return nil }
func (availableProjectLifecycle) Restart(context.Context, string) error             { return nil }
func (availableProjectLifecycle) Delete(context.Context, string) error              { return nil }
func (availableProjectLifecycle) State(context.Context, string) (serviceproject.ContainerState, error) {
	return serviceproject.ContainerStateMissing, nil
}
func (availableProjectLifecycle) EnsureResources(context.Context, string) error { return nil }
func (availableProjectLifecycle) SetResourceLimits(context.Context, string, serviceproject.ContainerLimits) error {
	return nil
}

func TestReconcileContinuesAcrossApplicationAndProjectFailures(t *testing.T) {
	applicationStore, err := fileapplications.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	applications := serviceapplications.New(
		emptyApplicationRegistry{},
		applicationStore,
		nil,
		nil,
		nil,
		serviceapplications.WithDefaultApplications(applicationStore, "missing-default"),
	)
	projectErr := errors.New("project list failed")
	projectRepo := &failingProjectListRepository{err: projectErr}
	projects := serviceproject.New(
		projectRepo,
		serviceproject.ContainerDependencies{Lifecycle: availableProjectLifecycle{}},
		nil,
		nil,
	)

	err = (Services{Applications: applications, Projects: projects}).Reconcile(context.Background())
	if !errors.Is(err, serviceapplications.ErrUnknownApplication) {
		t.Errorf("Reconcile error = %v, want application failure", err)
	}
	if !errors.Is(err, projectErr) {
		t.Errorf("Reconcile error = %v, want project failure", err)
	}
	if projectRepo.calls != 1 {
		t.Errorf("project reconcile calls = %d, want 1 after application failure", projectRepo.calls)
	}
}

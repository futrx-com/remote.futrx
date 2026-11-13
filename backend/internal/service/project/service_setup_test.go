package project

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
)

type setupFunc func(context.Context, string, map[string]string) error

func (f setupFunc) Ensure(ctx context.Context, name string, env map[string]string) error {
	return f(ctx, name, env)
}

type setupSecrets struct {
	SecretsRepository
	values []Secret
	err    error
}

func (s setupSecrets) List(context.Context, ID) ([]Secret, error) { return s.values, s.err }

type setupEnvironment struct{ calls *[]string }

func (e setupEnvironment) ApplyDiff(context.Context, string, map[string]string, []string) error {
	*e.calls = append(*e.calls, "environment")
	return nil
}

type setupLifecycle struct {
	*startTestLifecycle
	calls *[]string
}

func (l setupLifecycle) Ensure(ctx context.Context, meta Meta) error {
	*l.calls = append(*l.calls, "container")
	return l.startTestLifecycle.Ensure(ctx, meta)
}

type setupRepository struct{ *startTestRepository }

func (r setupRepository) Create(ctx context.Context, meta Meta) (Meta, error) {
	meta.ID, meta.ContainerName = "abcd", "project"
	return r.startTestRepository.Create(ctx, meta)
}

func TestProjectSetupReceivesSecretsAfterContainerPreparation(t *testing.T) {
	for _, operation := range []string{"create", "start", "upgrade"} {
		t.Run(operation, func(t *testing.T) {
			var calls []string
			repo := setupRepository{&startTestRepository{meta: Meta{ID: "abcd", ContainerName: "project", Status: StatusMissing}}}
			lifecycle := setupLifecycle{&startTestLifecycle{state: ContainerStateMissing}, &calls}
			setup := setupFunc(func(_ context.Context, name string, env map[string]string) error {
				calls = append(calls, "setup")
				if name != "project" || env["NPM_TOKEN"] != "test-private-token" || env["MULTILINE_KEY"] != "line1\nline2" {
					t.Fatalf("setup did not receive project environment: name=%q, keys=%d", name, len(env))
				}
				return nil
			})
			secrets := setupSecrets{values: []Secret{{Key: "NPM_TOKEN", Value: "test-private-token"}, {Key: "MULTILINE_KEY", Value: "line1\nline2"}}}
			service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Environment: setupEnvironment{&calls}, Setup: setup}, secrets, nil)
			var result Meta
			var err error
			switch operation {
			case "create":
				result, err = service.Create(rbac.ContextWithSystemActor(context.Background()), CreateInput{Name: "project"}, "")
			case "start":
				result, err = service.Start(rbac.ContextWithSystemActor(context.Background()), "abcd")
			case "upgrade":
				result, err = service.Upgrade(rbac.ContextWithSystemActor(context.Background()), "abcd", false)
			}
			if err != nil || result.Status != StatusRunning {
				t.Fatalf("%s = %+v, %v", operation, result, err)
			}
			if !slices.Equal(calls, []string{"container", "environment", "setup"}) {
				t.Fatalf("setup order = %q", calls)
			}
		})
	}
}

func TestSetupFailurePersistsAndStartOrRestartRetries(t *testing.T) {
	for _, retry := range []string{"start", "restart"} {
		t.Run(retry, func(t *testing.T) {
			repo := &startTestRepository{meta: Meta{ID: "abcd", ContainerName: "project", Status: StatusMissing}}
			lifecycle := &startTestLifecycle{state: ContainerStateMissing}
			want := errors.New("setup.sh failed: dependency unavailable")
			attempts := 0
			setup := setupFunc(func(context.Context, string, map[string]string) error {
				attempts++
				if attempts == 1 {
					return want
				}
				return nil
			})
			service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Setup: setup}, nil, nil)
			got, err := service.Start(rbac.ContextWithSystemActor(context.Background()), "abcd")
			if !errors.Is(err, want) || got.Status != StatusError || repo.meta.ErrorMsg != want.Error() {
				t.Fatalf("failure was not persisted: %+v, %v", got, err)
			}
			if lifecycle.state != ContainerStateRunning {
				t.Fatal("failed setup destroyed the repairable container")
			}
			if retry == "start" {
				got, err = service.Start(rbac.ContextWithSystemActor(context.Background()), "abcd")
			} else {
				got, err = service.Restart(rbac.ContextWithSystemActor(context.Background()), "abcd")
			}
			if err != nil || got.Status != StatusRunning || got.ErrorMsg != "" || attempts != 2 {
				t.Fatalf("retry = %+v, %v, attempts=%d", got, err, attempts)
			}
		})
	}
}

func TestSetupDoesNotRunWhenSecretLoadingFails(t *testing.T) {
	repo := &startTestRepository{meta: Meta{ID: "abcd", ContainerName: "project", Status: StatusRunning}}
	want := errors.New("secret store unavailable")
	setup := setupFunc(func(context.Context, string, map[string]string) error {
		t.Fatal("setup ran without its environment")
		return nil
	})
	service := New(repo, ContainerDependencies{Lifecycle: &startTestLifecycle{state: ContainerStateRunning}, Setup: setup}, setupSecrets{err: want}, nil)
	got, err := service.Start(rbac.ContextWithSystemActor(context.Background()), "abcd")
	if !errors.Is(err, want) || got.Status != StatusError {
		t.Fatalf("missing secret error = %+v, %v", got, err)
	}
}

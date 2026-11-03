package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

func TestUpgradeAllBudgetsEachWorkspaceIncludingAppRestoration(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		count                    int
		firstSetup, firstRestore time.Duration
		wantUpgraded, wantFailed int
	}{
		{"fleet exceeds thirty minutes", 23, 2 * time.Minute, time.Minute, 23, 0},
		{"setup timeout leaves next workspace usable", 2, 31 * time.Minute, time.Minute, 1, 1},
		{"restoration shares the workspace budget", 2, 20 * time.Minute, 11 * time.Minute, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := newUpgradeTestRepository(tc.count)
				var contexts []context.Context
				lifecycle := &upgradeTestLifecycle{ensure: func(ctx context.Context, meta serviceproject.Meta) error {
					contexts = append(contexts, ctx)
					if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) != 30*time.Minute {
						t.Fatalf("workspace %s did not receive a fresh 30-minute budget", meta.ID)
					}
					duration := 2 * time.Minute
					if meta.ID == "0000" {
						duration = tc.firstSetup
					}
					return waitForUpgrade(ctx, duration)
				}}
				projects := serviceproject.New(repo, serviceproject.ContainerDependencies{Lifecycle: lifecycle}, nil, nil)
				restore := func(ctx context.Context, id string) error {
					duration := time.Minute
					if id == "0000" {
						duration = tc.firstRestore
					}
					return waitForUpgrade(ctx, duration)
				}
				upgraded, skipped, setupFailed, failed, err := upgradeAll(context.Background(), projects, lifecycle, false, false, newProgressWriter(""), restore)
				if err != nil || upgraded != tc.wantUpgraded || skipped != 0 || setupFailed != 0 || failed != tc.wantFailed {
					t.Fatalf("upgradeAll = %d upgraded, %d skipped, %d failed, %v", upgraded, skipped, failed, err)
				}
				for _, ctx := range contexts {
					if ctx.Err() == nil {
						t.Fatal("completed workspace retained an active timeout")
					}
				}
			})
		})
	}
}

func TestUpgradeAllStopsAfterParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lifecycle := &upgradeTestLifecycle{ensure: func(context.Context, serviceproject.Meta) error { return nil }}
	projects := serviceproject.New(newUpgradeTestRepository(3), serviceproject.ContainerDependencies{Lifecycle: lifecycle}, nil, nil)
	restore := func(context.Context, string) error { cancel(); return nil }
	upgraded, skipped, setupFailed, failed, err := upgradeAll(ctx, projects, lifecycle, false, false, newProgressWriter(""), restore)
	if !errors.Is(err, context.Canceled) || upgraded != 1 || skipped != 0 || setupFailed != 0 || failed != 0 {
		t.Fatalf("upgradeAll = %d upgraded, %d skipped, %d failed, %v", upgraded, skipped, failed, err)
	}
}

func waitForUpgrade(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type upgradeTestLifecycle struct {
	serviceproject.ContainerLifecycle
	ensure func(context.Context, serviceproject.Meta) error
	states map[string]serviceproject.ContainerState
}

func (l *upgradeTestLifecycle) State(ctx context.Context, name string) (serviceproject.ContainerState, error) {
	if state, ok := l.states[name]; ok {
		return state, ctx.Err()
	}
	return serviceproject.ContainerStateMissing, ctx.Err()
}

func (l *upgradeTestLifecycle) Ensure(ctx context.Context, meta serviceproject.Meta) error {
	if err := l.ensure(ctx, meta); err != nil {
		return err
	}
	if l.states == nil {
		l.states = make(map[string]serviceproject.ContainerState)
	}
	l.states[meta.ContainerName] = serviceproject.ContainerStateRunning
	return nil
}

type upgradeTestRepository struct {
	serviceproject.Repository
	metas     []serviceproject.Meta
	statusErr error
}

func newUpgradeTestRepository(count int) *upgradeTestRepository {
	repo := &upgradeTestRepository{}
	for i := range count {
		id := fmt.Sprintf("%04x", i)
		repo.metas = append(repo.metas, serviceproject.Meta{ID: serviceproject.ID(id), Slug: id, ContainerName: id})
	}
	return repo
}

func (r *upgradeTestRepository) List(context.Context) ([]serviceproject.Meta, error) {
	return r.metas, nil
}

func (r *upgradeTestRepository) Get(_ context.Context, id serviceproject.ID) (serviceproject.Meta, error) {
	for _, meta := range r.metas {
		if meta.ID == id {
			return meta, nil
		}
	}
	return serviceproject.Meta{}, serviceproject.ErrNotFound
}

func (r *upgradeTestRepository) Update(ctx context.Context, id serviceproject.ID, update func(*serviceproject.Meta)) (serviceproject.Meta, error) {
	if r.statusErr != nil {
		return serviceproject.Meta{}, r.statusErr
	}
	meta, err := r.Get(ctx, id)
	if err != nil {
		return meta, err
	}
	update(&meta)
	for i := range r.metas {
		if r.metas[i].ID == id {
			r.metas[i] = meta
		}
	}
	return meta, nil
}

func (r *upgradeTestRepository) SetStatus(ctx context.Context, id serviceproject.ID, status serviceproject.Status, message string) (serviceproject.Meta, error) {
	if r.statusErr != nil {
		return serviceproject.Meta{}, r.statusErr
	}
	meta, err := r.Get(ctx, id)
	meta.Status, meta.ErrorMsg = status, message
	for i := range r.metas {
		if r.metas[i].ID == id {
			r.metas[i] = meta
		}
	}
	return meta, err
}

type upgradeTestSetup func(context.Context, string, map[string]string) error

func (f upgradeTestSetup) Ensure(ctx context.Context, name string, env map[string]string) error {
	return f(ctx, name, env)
}

func (f upgradeTestSetup) Verify(ctx context.Context, name string, env map[string]string) error {
	return f(ctx, name, env)
}

func TestUpgradeAllSeparatesSetupRepairFromInfrastructureFailure(t *testing.T) {
	for _, tc := range []struct {
		name                                      string
		setupErr                                  error
		stopped, restoreFails, persistFails       bool
		wantUpgraded, wantSetupFailed, wantFailed int
		wantRestored                              []string
	}{
		{"project script failed", serviceproject.ErrSetupFailed, false, false, false, 2, 1, 0, []string{"0001"}},
		{"setup transport failed", errors.New("LXD connection lost"), false, false, false, 1, 0, 1, []string{"0001"}},
		{"setup context expired", context.DeadlineExceeded, false, false, false, 1, 0, 1, []string{"0001"}},
		{"script stopped container", serviceproject.ErrSetupFailed, true, false, false, 1, 0, 1, []string{"0001"}},
		{"application restoration deferred until setup repair", serviceproject.ErrSetupFailed, false, true, false, 2, 1, 0, []string{"0001"}},
		{"application restoration failed after successful setup", nil, false, true, false, 1, 0, 1, []string{"0000", "0001"}},
		{"could not save project failure", serviceproject.ErrSetupFailed, false, false, true, 0, 0, 2, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newUpgradeTestRepository(2)
			if tc.persistFails {
				repo.statusErr = errors.New("project store unavailable")
			}
			lifecycle := &upgradeTestLifecycle{ensure: func(context.Context, serviceproject.Meta) error { return nil }}
			setup := upgradeTestSetup(func(_ context.Context, name string, _ map[string]string) error {
				if name != "0000" {
					return nil
				}
				if tc.stopped {
					lifecycle.states[name] = serviceproject.ContainerStateStopped
				}
				return tc.setupErr
			})
			projects := serviceproject.New(repo, serviceproject.ContainerDependencies{Lifecycle: lifecycle, Setup: setup}, nil, nil)
			var restored []string
			restore := func(_ context.Context, id string) error {
				restored = append(restored, id)
				if tc.restoreFails && id == "0000" {
					return errors.New("application install failed")
				}
				return nil
			}
			path := filepath.Join(t.TempDir(), "progress.json")
			upgraded, skipped, setupFailed, failed, err := upgradeAll(context.Background(), projects, lifecycle, false, false, newProgressWriter(path), restore)
			if err != nil || skipped != 0 || upgraded != tc.wantUpgraded || setupFailed != tc.wantSetupFailed || failed != tc.wantFailed {
				t.Fatalf("upgradeAll = upgraded=%d skipped=%d setup=%d failed=%d err=%v", upgraded, skipped, setupFailed, failed, err)
			}
			if !slices.Equal(restored, tc.wantRestored) {
				t.Fatalf("restored=%v, want=%v", restored, tc.wantRestored)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var progress workspaceProgress
			if err := json.Unmarshal(data, &progress); err != nil {
				t.Fatal(err)
			}
			if progress.Completed != 2 || len(progress.SetupFailures) != tc.wantSetupFailed {
				t.Fatalf("incomplete progress: %+v", progress)
			}
			if tc.wantSetupFailed > 0 {
				if progress.SetupFailures[0] != "0000" || repo.metas[0].Status != serviceproject.StatusError || repo.metas[0].ErrorMsg == "" {
					t.Fatalf("project repair details lost: progress=%+v project=%+v", progress, repo.metas[0])
				}
			}
		})
	}
}

func TestUpgradeDefersApplicationsUntilSetupRepairIsVerified(t *testing.T) {
	ctx := context.Background()
	repo := newUpgradeTestRepository(2)
	lifecycle := &upgradeTestLifecycle{ensure: func(context.Context, serviceproject.Meta) error { return nil }}
	repaired := false
	setup := upgradeTestSetup(func(_ context.Context, name string, _ map[string]string) error {
		if name == "0000" && !repaired {
			return serviceproject.ErrSetupFailed
		}
		return nil
	})
	projects := serviceproject.New(repo, serviceproject.ContainerDependencies{Lifecycle: lifecycle, Setup: setup}, nil, nil)
	var restored []string
	restore := func(_ context.Context, id string) error {
		if id == "0000" && !repaired {
			t.Fatal("restored applications before their setup dependencies were repaired")
		}
		restored = append(restored, id)
		return nil
	}
	projects.SetContainerRestorer(restore)
	upgraded, skipped, setupFailed, failed, err := upgradeAll(ctx, projects, lifecycle, false, false, newProgressWriter(""), restore)
	if err != nil || upgraded != 2 || skipped != 0 || setupFailed != 1 || failed != 0 || !slices.Equal(restored, []string{"0001"}) {
		t.Fatalf("upgrade result: upgraded=%d skipped=%d setup=%d failed=%d restored=%v err=%v", upgraded, skipped, setupFailed, failed, restored, err)
	}
	repaired = true
	meta, err := projects.VerifySetup(ctx, "0000")
	if err != nil || meta.Status != serviceproject.StatusRunning || meta.SetupFailed || !slices.Equal(restored, []string{"0001", "0000"}) {
		t.Fatalf("setup recovery did not restore deferred applications: project=%+v restored=%v err=%v", meta, restored, err)
	}
}

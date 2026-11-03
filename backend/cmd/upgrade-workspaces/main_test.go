package main

import (
	"context"
	"errors"
	"fmt"
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
				upgraded, skipped, failed, err := upgradeAll(context.Background(), projects, lifecycle, false, false, newProgressWriter(""), restore)
				if err != nil || upgraded != tc.wantUpgraded || skipped != 0 || failed != tc.wantFailed {
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
	upgraded, skipped, failed, err := upgradeAll(ctx, projects, lifecycle, false, false, newProgressWriter(""), restore)
	if !errors.Is(err, context.Canceled) || upgraded != 1 || skipped != 0 || failed != 0 {
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
}

func (l *upgradeTestLifecycle) State(ctx context.Context, _ string) (serviceproject.ContainerState, error) {
	return serviceproject.ContainerStateMissing, ctx.Err()
}

func (l *upgradeTestLifecycle) Ensure(ctx context.Context, meta serviceproject.Meta) error {
	return l.ensure(ctx, meta)
}

type upgradeTestRepository struct {
	serviceproject.Repository
	metas []serviceproject.Meta
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

func (r *upgradeTestRepository) SetStatus(ctx context.Context, id serviceproject.ID, status serviceproject.Status, message string) (serviceproject.Meta, error) {
	meta, err := r.Get(ctx, id)
	meta.Status, meta.ErrorMsg = status, message
	return meta, err
}

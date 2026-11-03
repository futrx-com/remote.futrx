package project

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAgentStartBypassesOnlyConfirmedSetupFailure(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		repo := &startTestRepository{meta: Meta{ID: "abcd", ContainerName: "project", Status: StatusError, SetupFailed: !legacy, ErrorMsg: "project setup failed: missing dependency"}}
		lifecycle := &startTestLifecycle{state: ContainerStateRunning}
		setup := setupFunc(func(context.Context, string, map[string]string) error {
			t.Fatal("agent launch reran broken setup")
			return nil
		})
		service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Setup: setup}, nil, nil)
		got, err := service.StartForAgent(context.Background(), "abcd")
		if err != nil || !got.NeedsSetupRepair() {
			t.Fatalf("agent was blocked or error hidden: %+v %v", got, err)
		}
		lifecycle.launchErr = errors.New("mount validation failed")
		if _, err := service.StartForAgent(context.Background(), "abcd"); !errors.Is(err, lifecycle.launchErr) {
			t.Fatalf("infrastructure failure bypassed: %v", err)
		}
	}
}

func TestAgentStartAllowsFirstSetupFailureButNotStoppedContainer(t *testing.T) {
	for _, stop := range []bool{false, true} {
		repo := &startTestRepository{meta: Meta{ID: "abcd", ContainerName: "project", Status: StatusRunning}}
		lifecycle := &startTestLifecycle{state: ContainerStateRunning}
		service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Setup: setupFunc(func(context.Context, string, map[string]string) error {
			if stop {
				lifecycle.state = ContainerStateStopped
			}
			return ErrSetupFailed
		})}, nil, nil)
		got, err := service.StartForAgent(context.Background(), "abcd")
		if (err != nil) != stop || !got.SetupFailed {
			t.Fatalf("stop=%v got=%+v err=%v", stop, got, err)
		}
	}
}

type verifyingSetup struct{ verify func() error }

func (v verifyingSetup) Ensure(context.Context, string, map[string]string) error {
	panic("verification used cached Ensure")
}
func (v verifyingSetup) Verify(context.Context, string, map[string]string) error { return v.verify() }

func TestSetupRepairIsClaimedOnceAndVerifiedBeforeClearingError(t *testing.T) {
	repo := &startTestRepository{meta: Meta{ID: "abcd", ContainerName: "project", Status: StatusError, SetupFailed: true, ErrorMsg: "broken"}}
	lifecycle := &startTestLifecycle{state: ContainerStateRunning}
	verified := false
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Setup: verifyingSetup{func() error { verified = true; return nil }}}, nil, nil)
	restored := false
	service.SetContainerRestorer(func(context.Context, string) error {
		if !verified {
			t.Fatal("restored before verification")
		}
		restored = true
		return nil
	})
	for i := 0; i < 2; i++ {
		claimed, err := service.ClaimSetupRepair(context.Background(), "abcd", "cafe")
		if err != nil || claimed != (i == 0) {
			t.Fatalf("claim %d=%v,%v", i, claimed, err)
		}
	}
	got, err := service.VerifySetup(context.Background(), "abcd")
	if err != nil || got.Status != StatusRunning || got.SetupFailed || !restored || got.SetupRepair.State != "succeeded" {
		t.Fatalf("verified=%+v, %v", got, err)
	}
	if lifecycle.launchCalls != 0 || lifecycle.restartCalls != 0 {
		t.Fatal("repair restarted or recreated container")
	}
}

func TestRepairDoesNotClaimBusyOrStoppedProjects(t *testing.T) {
	for _, state := range []ContainerState{ContainerStateRunning, ContainerStateStopped} {
		repo := &startTestRepository{meta: Meta{ID: "abcd", ContainerName: "project", Status: StatusError, SetupFailed: true}}
		service := New(repo, ContainerDependencies{Lifecycle: &startTestLifecycle{state: state, busy: true}}, nil, nil)
		claimed, err := service.ClaimSetupRepair(context.Background(), "abcd", "cafe")
		if err != nil || claimed || repo.meta.SetupRepair != nil {
			t.Fatalf("claimed busy/stopped project: %v %v", claimed, err)
		}
	}
}

func TestFailedVerificationRetainsError(t *testing.T) {
	repo := &startTestRepository{meta: Meta{ID: "abcd", ContainerName: "project", Status: StatusError, SetupFailed: true}}
	service := New(repo, ContainerDependencies{Lifecycle: &startTestLifecycle{state: ContainerStateRunning}, Setup: verifyingSetup{func() error { return ErrSetupFailed }}}, nil, nil)
	service.SetContainerRestorer(func(context.Context, string) error { t.Fatal("restored despite failed verification"); return nil })
	got, err := service.VerifySetup(context.Background(), "abcd")
	if !errors.Is(err, ErrSetupFailed) || !got.NeedsSetupRepair() {
		t.Fatalf("failure hidden: %+v %v", got, err)
	}
}

func TestCanceledVerificationPreservesFailure(t *testing.T) {
	for _, phase := range []string{"before", "setup error", "setup success", "restore error", "restore success"} {
		t.Run(phase, func(t *testing.T) {
			original := Meta{ID: "abcd", ContainerName: "project", Status: StatusError, SetupFailed: true, ErrorMsg: "missing dependency", SetupRepair: &SetupRepair{ChatID: "cafe", State: "running"}}
			repo := &startTestRepository{meta: original}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			service := New(repo, ContainerDependencies{Lifecycle: &startTestLifecycle{state: ContainerStateRunning}, Setup: verifyingSetup{func() error {
				if phase == "before" {
					t.Fatal("verification started after cancellation")
				}
				if phase == "setup error" || phase == "setup success" {
					cancel()
				}
				if phase == "setup error" {
					return ErrSetupFailed
				}
				return nil
			}}}, nil, nil)
			service.SetContainerRestorer(func(context.Context, string) error {
				if phase != "restore error" && phase != "restore success" {
					t.Fatal("restoration started after cancellation")
				}
				cancel()
				if phase == "restore error" {
					return context.Canceled
				}
				return nil
			})
			if phase == "before" {
				cancel()
			}
			if _, err := service.VerifySetup(ctx, "abcd"); err == nil {
				t.Fatal("canceled verification succeeded")
			}
			if !reflect.DeepEqual(repo.meta, original) {
				t.Fatalf("cancellation replaced the setup failure: %+v", repo.meta)
			}
		})
	}
}

func TestStaleRepairProgressPreservesCurrentState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current Meta
		next    *SetupRepair
	}{
		{"blocked after recovery", Meta{Status: StatusRunning}, &SetupRepair{State: "blocked"}},
		{"blocked after claim", Meta{Status: StatusError, SetupFailed: true, SetupRepair: &SetupRepair{ChatID: "cafe", State: "running"}}, &SetupRepair{State: "blocked"}},
		{"old claim completed", Meta{Status: StatusError, SetupFailed: true, SetupRepair: &SetupRepair{ChatID: "beef", State: "running"}}, &SetupRepair{ChatID: "cafe", State: "failed"}},
		{"failure after recovery", Meta{Status: StatusRunning, SetupRepair: &SetupRepair{ChatID: "cafe", State: "succeeded"}}, &SetupRepair{ChatID: "cafe", State: "failed"}},
		{"released after recovery", Meta{Status: StatusRunning, SetupRepair: &SetupRepair{ChatID: "cafe", State: "succeeded"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &startTestRepository{meta: tc.current}
			service := New(repo, ContainerDependencies{}, nil, nil)
			if err := service.SetSetupRepair(context.Background(), "abcd", tc.next); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(repo.meta, tc.current) {
				t.Fatalf("stale progress replaced current state: %+v", repo.meta)
			}
		})
	}
}

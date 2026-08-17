package project

import (
	"context"
	"errors"
	"testing"
)

type stubPolicy struct {
	snapshot ContainerPolicySnapshot
	rejected error
	seen     []ContainerLimits
}

func (p *stubPolicy) Policy(context.Context) ContainerPolicySnapshot { return p.snapshot }

func (p *stubPolicy) Validate(_ context.Context, limits ContainerLimits) error {
	p.seen = append(p.seen, limits)
	return p.rejected
}

type stubAdmission struct {
	denied error
	calls  []string
	forced []bool
	memory []string
}

func (a *stubAdmission) AuthorizeStart(_ context.Context, container, memory string, force bool) error {
	a.calls = append(a.calls, container)
	a.memory = append(a.memory, memory)
	a.forced = append(a.forced, force)
	if force {
		return nil
	}
	return a.denied
}

type recordingLifecycle struct {
	*startTestLifecycle
	ensured []Meta
	limits  []ContainerLimits
}

func (l *recordingLifecycle) Ensure(ctx context.Context, meta Meta) error {
	l.ensured = append(l.ensured, meta)
	return l.startTestLifecycle.Ensure(ctx, meta)
}

func (l *recordingLifecycle) SetResourceLimits(_ context.Context, _ string, limits ContainerLimits) error {
	l.limits = append(l.limits, limits)
	return nil
}

func fleetPolicy() *stubPolicy {
	return &stubPolicy{snapshot: ContainerPolicySnapshot{
		Defaults:    ContainerLimits{CPU: "2", Memory: "2GiB", Disk: "20GiB"},
		MaxOverride: ContainerLimits{CPU: "4", Memory: "3GiB", Disk: "40GiB"},
		Host:        HostCapacity{MemoryBytes: 8 << 30, CPUs: 4},
		DiskQuota:   DiskQuotaSupport{Supported: true, Pool: "default", Driver: "zfs"},
	}}
}

func TestEffectiveLimitsMergeOverridesOverFleetDefaults(t *testing.T) {
	service := New(&startTestRepository{}, ContainerDependencies{Policy: fleetPolicy()}, nil, nil)
	ctx := context.Background()

	tests := []struct {
		name      string
		overrides *ContainerLimits
		want      ContainerLimits
	}{
		{
			name: "no override inherits every default",
			want: ContainerLimits{CPU: "2", Memory: "2GiB", Disk: "20GiB"},
		},
		{
			name:      "empty override inherits every default",
			overrides: &ContainerLimits{},
			want:      ContainerLimits{CPU: "2", Memory: "2GiB", Disk: "20GiB"},
		},
		{
			name:      "memory-only override keeps the other defaults",
			overrides: &ContainerLimits{Memory: "3GiB"},
			want:      ContainerLimits{CPU: "2", Memory: "3GiB", Disk: "20GiB"},
		},
		{
			name:      "full override replaces every default",
			overrides: &ContainerLimits{CPU: "4", Memory: "3GiB", Disk: "40GiB"},
			want:      ContainerLimits{CPU: "4", Memory: "3GiB", Disk: "40GiB"},
		},
		{
			name:      "whitespace counts as unset",
			overrides: &ContainerLimits{CPU: "   ", Memory: "3GiB"},
			want:      ContainerLimits{CPU: "2", Memory: "3GiB", Disk: "20GiB"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := service.effectiveLimits(ctx, test.overrides); got != test.want {
				t.Fatalf("effectiveLimits = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestEffectiveLimitsWithoutAPolicySourceStayEmpty(t *testing.T) {
	service := New(&startTestRepository{}, ContainerDependencies{}, nil, nil)

	if got := service.effectiveLimits(context.Background(), nil); got != (ContainerLimits{}) {
		t.Fatalf("effectiveLimits = %+v, want an empty envelope", got)
	}
}

func TestStartAppliesTheResolvedEnvelopeToTheLifecycle(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "p", ContainerName: "p",
		Status:         StatusStopped,
		ResourceLimits: &ContainerLimits{Memory: "3GiB"},
	}}
	lifecycle := &recordingLifecycle{startTestLifecycle: &startTestLifecycle{state: ContainerStateStopped}}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Policy: fleetPolicy()}, nil, nil)

	if _, err := service.Start(context.Background(), repo.meta.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(lifecycle.ensured) != 1 {
		t.Fatalf("Ensure calls = %d, want 1", len(lifecycle.ensured))
	}
	got := lifecycle.ensured[0].ResourceLimits
	if got == nil || *got != (ContainerLimits{CPU: "2", Memory: "3GiB", Disk: "20GiB"}) {
		t.Fatalf("launched envelope = %+v, want the fleet default merged under the override", got)
	}
	// The stored project must keep only its explicit override.
	if repo.meta.ResourceLimits == nil || *repo.meta.ResourceLimits != (ContainerLimits{Memory: "3GiB"}) {
		t.Fatalf("stored override = %+v, want the untouched project override", repo.meta.ResourceLimits)
	}
}

func TestStartConsultsTheAggregateGuard(t *testing.T) {
	denial := errors.New("host memory budget exhausted")
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "p", ContainerName: "p", Status: StatusStopped,
	}}
	lifecycle := &recordingLifecycle{startTestLifecycle: &startTestLifecycle{state: ContainerStateStopped}}
	admission := &stubAdmission{denied: denial}
	service := New(repo, ContainerDependencies{
		Lifecycle: lifecycle, Policy: fleetPolicy(), Admission: admission,
	}, nil, nil)

	if _, err := service.Start(context.Background(), repo.meta.ID); !errors.Is(err, denial) {
		t.Fatalf("Start error = %v, want the admission denial", err)
	}
	if len(lifecycle.ensured) != 0 {
		t.Fatal("a refused start must not reach the lifecycle")
	}
	if len(admission.memory) != 1 || admission.memory[0] != "2GiB" {
		t.Fatalf("guard saw memory %v, want the resolved fleet default", admission.memory)
	}
	// A denial is a capacity answer, not a broken project: the stored status
	// must not flip to error.
	if repo.meta.Status == StatusError {
		t.Fatal("an admission denial must not mark the project as errored")
	}
}

func TestForceStartBypassesTheAggregateGuard(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "p", ContainerName: "p", Status: StatusStopped,
	}}
	lifecycle := &recordingLifecycle{startTestLifecycle: &startTestLifecycle{state: ContainerStateStopped}}
	admission := &stubAdmission{denied: errors.New("host memory budget exhausted")}
	service := New(repo, ContainerDependencies{
		Lifecycle: lifecycle, Policy: fleetPolicy(), Admission: admission,
	}, nil, nil)

	if _, err := service.StartWithOptions(context.Background(), repo.meta.ID, StartOptions{Force: true}); err != nil {
		t.Fatalf("forced Start: %v", err)
	}
	if len(lifecycle.ensured) != 1 {
		t.Fatalf("Ensure calls = %d, want the forced start to proceed", len(lifecycle.ensured))
	}
	if len(admission.forced) != 1 || !admission.forced[0] {
		t.Fatalf("guard force flag = %v, want a single forced call", admission.forced)
	}
}

func TestStartOfARunningContainerSkipsTheGuard(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "p", ContainerName: "p", Status: StatusRunning,
	}}
	lifecycle := &recordingLifecycle{startTestLifecycle: &startTestLifecycle{state: ContainerStateRunning}}
	admission := &stubAdmission{denied: errors.New("host memory budget exhausted")}
	service := New(repo, ContainerDependencies{
		Lifecycle: lifecycle, Policy: fleetPolicy(), Admission: admission,
	}, nil, nil)

	if _, err := service.Start(context.Background(), repo.meta.ID); err != nil {
		t.Fatalf("Start of a running container: %v", err)
	}
	if len(admission.calls) != 0 {
		t.Fatalf("guard calls = %v, want none for an already-running container", admission.calls)
	}
}

func TestSetResourcesRejectsAnOverrideAboveTheCeiling(t *testing.T) {
	policy := fleetPolicy()
	policy.rejected = errors.New("resource override exceeds the allowed maximum")
	repo := &startTestRepository{meta: Meta{ID: ID("abcd"), Name: "p", ContainerName: "p"}}
	lifecycle := &recordingLifecycle{startTestLifecycle: &startTestLifecycle{state: ContainerStateRunning}}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Policy: policy}, nil, nil)

	_, err := service.SetResources(context.Background(), repo.meta.ID, ContainerLimits{Memory: "8GiB"})
	if !errors.Is(err, policy.rejected) {
		t.Fatalf("SetResources error = %v, want the policy rejection", err)
	}
	if len(lifecycle.limits) != 0 {
		t.Fatal("a rejected override must not reach the container")
	}
	if repo.meta.ResourceLimits != nil {
		t.Fatalf("a rejected override must not be persisted, got %+v", repo.meta.ResourceLimits)
	}
}

func TestSetResourcesRejectsMalformedValues(t *testing.T) {
	repo := &startTestRepository{meta: Meta{ID: ID("abcd"), Name: "p", ContainerName: "p"}}
	service := New(repo, ContainerDependencies{Policy: fleetPolicy()}, nil, nil)

	for _, limits := range []ContainerLimits{
		{CPU: "two"},
		{CPU: "0"},
		{Memory: "8 gigs"},
		{Disk: "40G"},
	} {
		if _, err := service.SetResources(context.Background(), repo.meta.ID, limits); !errors.Is(err, ErrInvalidLimits) {
			t.Fatalf("SetResources(%+v) error = %v, want ErrInvalidLimits", limits, err)
		}
	}
}

func TestSetResourcesAppliesTheResolvedEnvelopeAndReportsRestartNeed(t *testing.T) {
	repo := &startTestRepository{meta: Meta{ID: ID("abcd"), Name: "p", ContainerName: "p"}}
	lifecycle := &recordingLifecycle{startTestLifecycle: &startTestLifecycle{state: ContainerStateRunning}}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Policy: fleetPolicy()}, nil, nil)

	got, err := service.SetResources(context.Background(), repo.meta.ID, ContainerLimits{Memory: "3GiB", Disk: "40GiB"})
	if err != nil {
		t.Fatalf("SetResources: %v", err)
	}

	if len(lifecycle.limits) != 1 || lifecycle.limits[0] != (ContainerLimits{CPU: "2", Memory: "3GiB", Disk: "40GiB"}) {
		t.Fatalf("applied limits = %+v, want the resolved envelope", lifecycle.limits)
	}
	if !got.AppliedNow {
		t.Fatal("appliedNow = false for a live container")
	}
	if !got.NeedsRestart {
		t.Fatal("needsRestart = false after a disk-quota change on a running container")
	}
	if repo.meta.ResourceLimits == nil || *repo.meta.ResourceLimits != (ContainerLimits{Memory: "3GiB", Disk: "40GiB"}) {
		t.Fatalf("persisted override = %+v", repo.meta.ResourceLimits)
	}
}

func TestSetResourcesWithoutADiskChangeNeedsNoRestart(t *testing.T) {
	repo := &startTestRepository{meta: Meta{ID: ID("abcd"), Name: "p", ContainerName: "p"}}
	lifecycle := &recordingLifecycle{startTestLifecycle: &startTestLifecycle{state: ContainerStateRunning}}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Policy: fleetPolicy()}, nil, nil)

	got, err := service.SetResources(context.Background(), repo.meta.ID, ContainerLimits{Memory: "3GiB"})
	if err != nil {
		t.Fatalf("SetResources: %v", err)
	}
	if got.NeedsRestart {
		t.Fatal("needsRestart = true although the disk quota is unchanged")
	}
}

func TestSetResourcesOnAMissingContainerOnlyPersists(t *testing.T) {
	repo := &startTestRepository{meta: Meta{ID: ID("abcd"), Name: "p", ContainerName: "p"}}
	lifecycle := &recordingLifecycle{startTestLifecycle: &startTestLifecycle{state: ContainerStateMissing}}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Policy: fleetPolicy()}, nil, nil)

	got, err := service.SetResources(context.Background(), repo.meta.ID, ContainerLimits{Memory: "3GiB"})
	if err != nil {
		t.Fatalf("SetResources: %v", err)
	}
	if len(lifecycle.limits) != 0 {
		t.Fatal("a missing container must not be configured")
	}
	if got.AppliedNow || got.NeedsRestart {
		t.Fatalf("appliedNow=%t needsRestart=%t, want both false", got.AppliedNow, got.NeedsRestart)
	}
	if repo.meta.ResourceLimits == nil {
		t.Fatal("the desired override must still be persisted for the next launch")
	}
}

func TestSetResourcesWithEveryFieldBlankClearsTheOverride(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "p", ContainerName: "p",
		ResourceLimits: &ContainerLimits{Memory: "3GiB"},
	}}
	lifecycle := &recordingLifecycle{startTestLifecycle: &startTestLifecycle{state: ContainerStateRunning}}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Policy: fleetPolicy()}, nil, nil)

	if _, err := service.SetResources(context.Background(), repo.meta.ID, ContainerLimits{}); err != nil {
		t.Fatalf("SetResources: %v", err)
	}
	if repo.meta.ResourceLimits != nil {
		t.Fatalf("override = %+v, want it cleared", repo.meta.ResourceLimits)
	}
	// Clearing the override reapplies the fleet default rather than removing
	// every limit from the container.
	if len(lifecycle.limits) != 1 || lifecycle.limits[0] != (ContainerLimits{CPU: "2", Memory: "2GiB", Disk: "20GiB"}) {
		t.Fatalf("applied limits = %+v, want the fleet default", lifecycle.limits)
	}
}

func TestResourcesReportsPolicyAndEditability(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "p", ContainerName: "p",
		ResourceLimits: &ContainerLimits{Memory: "3GiB"},
	}}
	service := New(repo, ContainerDependencies{Policy: fleetPolicy()}, nil, nil)

	member, err := service.Resources(context.Background(), repo.meta.ID, false)
	if err != nil {
		t.Fatalf("Resources: %v", err)
	}
	if member.Editable {
		t.Fatal("a member must not be told the form is editable")
	}
	if member.Policy.Defaults.Memory != "2GiB" || member.Policy.MaxOverride.Memory != "3GiB" {
		t.Fatalf("policy = %+v", member.Policy)
	}
	if member.Effective != (ContainerLimits{CPU: "2", Memory: "3GiB", Disk: "20GiB"}) {
		t.Fatalf("effective = %+v", member.Effective)
	}
	if member.Overrides == nil || member.Overrides.Memory != "3GiB" {
		t.Fatalf("overrides = %+v", member.Overrides)
	}

	admin, err := service.Resources(context.Background(), repo.meta.ID, true)
	if err != nil {
		t.Fatalf("Resources: %v", err)
	}
	if !admin.Editable {
		t.Fatal("an admin must be told the form is editable")
	}
}

func TestResourcesRejectsAnInvalidID(t *testing.T) {
	service := New(&startTestRepository{}, ContainerDependencies{Policy: fleetPolicy()}, nil, nil)

	if _, err := service.Resources(context.Background(), ID("x"), true); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("Resources error = %v, want ErrInvalidID", err)
	}
	if _, err := service.SetResources(context.Background(), ID("x"), ContainerLimits{}); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("SetResources error = %v, want ErrInvalidID", err)
	}
}

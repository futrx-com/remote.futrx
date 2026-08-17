package resources

import (
	"context"
	"errors"
	"testing"
)

type memoryRepository struct {
	settings  Settings
	found     bool
	saved     []Settings
	loadErr   error
	saveError error
}

func (r *memoryRepository) Load(context.Context) (Settings, bool, error) {
	if r.loadErr != nil {
		return Settings{}, false, r.loadErr
	}
	return r.settings, r.found, nil
}

func (r *memoryRepository) Save(_ context.Context, settings Settings) error {
	if r.saveError != nil {
		return r.saveError
	}
	r.settings, r.found = settings, true
	r.saved = append(r.saved, settings)
	return nil
}

type staticHost HostFacts

func (h staticHost) Facts(context.Context) HostFacts { return HostFacts(h) }

type stubFleet struct {
	available bool
	instances []Instance
	listErr   error
	pool      PoolCapability
	applied   []Limits
	applyErr  error
}

func (f *stubFleet) Available() bool { return f.available }

func (f *stubFleet) ApplyDefaults(_ context.Context, defaults Limits) error {
	f.applied = append(f.applied, defaults)
	return f.applyErr
}

func (f *stubFleet) PoolCapability(context.Context) (PoolCapability, error) { return f.pool, nil }

func (f *stubFleet) RunningInstances(context.Context) ([]Instance, error) {
	return f.instances, f.listErr
}

func hostOf(memory uint64, cpus int) staticHost {
	return staticHost{MemoryBytes: memory, CPUs: cpus, DiskBytes: 100 * gib}
}

func TestEnsureDerivesAndPersistsOnFirstRun(t *testing.T) {
	repo := &memoryRepository{}
	fleet := &stubFleet{available: true}
	service := New(repo, hostOf(4*gib, 1), fleet)

	if err := service.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if len(repo.saved) != 1 {
		t.Fatalf("expected the derived document to be persisted once, got %d writes", len(repo.saved))
	}
	if got := repo.saved[0].Defaults.Memory; got != "3GiB" {
		t.Fatalf("persisted memory default = %q, want 3GiB", got)
	}
	if len(fleet.applied) != 1 || fleet.applied[0].Memory != "3GiB" {
		t.Fatalf("fleet convergence = %+v, want one 3GiB apply", fleet.applied)
	}
}

func TestEnsureKeepsAnExistingDocument(t *testing.T) {
	repo := &memoryRepository{
		found: true,
		settings: Settings{
			Defaults:    Limits{Memory: "2GiB", CPU: 2, Processes: 2000, Disk: "20GiB"},
			HostReserve: DefaultReserve(),
		},
	}
	fleet := &stubFleet{available: true}
	service := New(repo, hostOf(4*gib, 1), fleet)

	if err := service.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(repo.saved) != 0 {
		t.Fatalf("an existing document must not be rewritten, got %d writes", len(repo.saved))
	}
	if len(fleet.applied) != 1 || fleet.applied[0].Memory != "2GiB" {
		t.Fatalf("fleet convergence = %+v, want one 2GiB apply", fleet.applied)
	}
}

func TestValidate(t *testing.T) {
	valid := Settings{
		Defaults:           Limits{Memory: "2GiB", CPU: 2, Processes: 2000, Disk: "20GiB"},
		HostReserve:        Reserve{Memory: "768MiB", CPU: 0.5},
		MaxProjectOverride: Limits{Memory: "3GiB", CPU: 4, Disk: "40GiB"},
	}
	mutate := func(fn func(*Settings)) Settings {
		out := valid
		fn(&out)
		return out
	}

	tests := []struct {
		name     string
		settings Settings
		facts    HostFacts
		wantErr  bool
	}{
		{name: "valid document", settings: valid, facts: HostFacts{MemoryBytes: 4 * gib, CPUs: 2}},
		{name: "no host facts still validates", settings: valid},
		{
			name:     "memory default is required",
			settings: mutate(func(s *Settings) { s.Defaults.Memory = "" }),
			wantErr:  true,
		},
		{
			name:     "memory default must parse",
			settings: mutate(func(s *Settings) { s.Defaults.Memory = "lots" }),
			wantErr:  true,
		},
		{
			name:     "memory default has a floor",
			settings: mutate(func(s *Settings) { s.Defaults.Memory = "64MiB" }),
			wantErr:  true,
		},
		{
			name:     "cpu default must be at least one",
			settings: mutate(func(s *Settings) { s.Defaults.CPU = 0 }),
			wantErr:  true,
		},
		{
			name:     "process default must be positive",
			settings: mutate(func(s *Settings) { s.Defaults.Processes = 0 }),
			wantErr:  true,
		},
		{
			name:     "disk default must parse",
			settings: mutate(func(s *Settings) { s.Defaults.Disk = "20 gigs" }),
			wantErr:  true,
		},
		{
			name:     "empty disk default means no quota",
			settings: mutate(func(s *Settings) { s.Defaults.Disk = "" }),
		},
		{
			name:     "override ceiling cannot sit below the default",
			settings: mutate(func(s *Settings) { s.MaxProjectOverride.Memory = "1GiB" }),
			wantErr:  true,
		},
		{
			name:     "cpu ceiling cannot sit below the default",
			settings: mutate(func(s *Settings) { s.MaxProjectOverride.CPU = 1 }),
			wantErr:  true,
		},
		{
			name:     "disk ceiling cannot sit below the default",
			settings: mutate(func(s *Settings) { s.MaxProjectOverride.Disk = "10GiB" }),
			wantErr:  true,
		},
		{
			name:     "running cap cannot be negative",
			settings: mutate(func(s *Settings) { s.MaxRunningContainers = -1 }),
			wantErr:  true,
		},
		{
			name:     "reserve cannot swallow the host",
			settings: mutate(func(s *Settings) { s.HostReserve.Memory = "8GiB" }),
			facts:    HostFacts{MemoryBytes: 4 * gib, CPUs: 2},
			wantErr:  true,
		},
		{
			name:     "default cannot exceed what the reserve leaves",
			settings: mutate(func(s *Settings) { s.Defaults.Memory = "4GiB" }),
			facts:    HostFacts{MemoryBytes: 4 * gib, CPUs: 2},
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.settings, test.facts)
			if test.wantErr && err == nil {
				t.Fatalf("Validate = nil, want an error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if test.wantErr && !errors.Is(err, ErrInvalidSettings) {
				t.Fatalf("Validate error = %v, want ErrInvalidSettings", err)
			}
		})
	}
}

func TestUpdateKeepsUnsubmittedFields(t *testing.T) {
	repo := &memoryRepository{
		found: true,
		settings: Settings{
			Defaults:           Limits{Memory: "2GiB", CPU: 2, Processes: 2000, Disk: "20GiB"},
			HostReserve:        Reserve{Memory: "768MiB", CPU: 0.5},
			MaxProjectOverride: Limits{Memory: "3GiB", CPU: 4, Disk: "40GiB"},
		},
	}
	service := New(repo, hostOf(8*gib, 4), &stubFleet{available: true})

	view, err := service.Update(context.Background(), Settings{
		Defaults:             Limits{Memory: "3GiB"},
		MaxRunningContainers: 3,
	}, "Admin@Example.com")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := view.Settings
	if got.Defaults.Memory != "3GiB" {
		t.Fatalf("memory = %q, want 3GiB", got.Defaults.Memory)
	}
	if got.Defaults.CPU != 2 || got.Defaults.Processes != 2000 || got.Defaults.Disk != "20GiB" {
		t.Fatalf("unsubmitted defaults were lost: %+v", got.Defaults)
	}
	if got.HostReserve.Memory != "768MiB" {
		t.Fatalf("reserve was lost: %+v", got.HostReserve)
	}
	if got.MaxRunningContainers != 3 {
		t.Fatalf("maxRunningContainers = %d, want 3", got.MaxRunningContainers)
	}
	if got.UpdatedBy != "admin@example.com" {
		t.Fatalf("updatedBy = %q, want a normalized email", got.UpdatedBy)
	}
	if got.Derived {
		t.Fatalf("an operator edit must clear the derived flag")
	}
}

func TestUpdateRejectsAnInvalidDocument(t *testing.T) {
	repo := &memoryRepository{found: true, settings: Settings{
		Defaults:    Limits{Memory: "2GiB", CPU: 2, Processes: 2000},
		HostReserve: DefaultReserve(),
	}}
	service := New(repo, hostOf(4*gib, 2), &stubFleet{available: true})

	if _, err := service.Update(context.Background(), Settings{Defaults: Limits{Memory: "64MiB"}}, "a@b.c"); err == nil {
		t.Fatal("Update accepted a memory default below the floor")
	}
	if len(repo.saved) != 0 {
		t.Fatalf("a rejected update must not be persisted, got %d writes", len(repo.saved))
	}
}

func policyService(fleet *stubFleet, maxRunning int) *Service {
	repo := &memoryRepository{found: true, settings: Settings{
		Defaults:             Limits{Memory: "2GiB", CPU: 2, Processes: 2000, Disk: "20GiB"},
		HostReserve:          Reserve{Memory: "768MiB", CPU: 0.5},
		MaxProjectOverride:   Limits{Memory: "3GiB", CPU: 4, Disk: "40GiB"},
		MaxRunningContainers: maxRunning,
	}}
	return New(repo, hostOf(8*gib, 4), fleet)
}

func TestAuthorizeStartAggregateMemoryGuard(t *testing.T) {
	// Budget on the fixture host: 8GiB - 768MiB = 7424MiB.
	tests := []struct {
		name       string
		instances  []Instance
		candidate  string
		container  string
		maxRunning int
		force      bool
		wantErr    error
	}{
		{
			name:      "empty host admits the first workspace",
			candidate: "2GiB",
			container: "alpha",
		},
		{
			name: "sum stays inside the budget",
			instances: []Instance{
				{Name: "alpha", Running: true, Memory: "2GiB"},
				{Name: "beta", Running: true, Memory: "2GiB"},
			},
			candidate: "2GiB",
			container: "gamma",
		},
		{
			name: "stopped containers do not count",
			instances: []Instance{
				{Name: "alpha", Running: false, Memory: "4GiB"},
				{Name: "beta", Running: false, Memory: "4GiB"},
			},
			candidate: "3GiB",
			container: "gamma",
		},
		{
			name: "restarting a running container does not double count",
			instances: []Instance{
				{Name: "alpha", Running: true, Memory: "4GiB"},
				{Name: "beta", Running: true, Memory: "3GiB"},
			},
			candidate: "4GiB",
			container: "alpha",
		},
		{
			name: "an instance without a ceiling counts as the fleet default",
			instances: []Instance{
				{Name: "alpha", Running: true},
				{Name: "beta", Running: true},
			},
			candidate: "3GiB",
			container: "gamma",
		},
		{
			name: "three default-sized instances plus a fourth overflow the budget",
			instances: []Instance{
				{Name: "alpha", Running: true},
				{Name: "beta", Running: true},
				{Name: "gamma", Running: true},
			},
			candidate: "2GiB",
			container: "delta",
			wantErr:   ErrHostMemoryExhausted,
		},
		{
			name: "oversubscription is refused",
			instances: []Instance{
				{Name: "alpha", Running: true, Memory: "4GiB"},
				{Name: "beta", Running: true, Memory: "2GiB"},
			},
			candidate: "2GiB",
			container: "gamma",
			wantErr:   ErrHostMemoryExhausted,
		},
		{
			name: "an admin can force past the guard",
			instances: []Instance{
				{Name: "alpha", Running: true, Memory: "6GiB"},
			},
			candidate: "3GiB",
			container: "gamma",
			force:     true,
		},
		{
			name:      "a blank candidate falls back to the fleet default",
			instances: []Instance{{Name: "alpha", Running: true, Memory: "6GiB"}},
			candidate: "",
			container: "gamma",
			wantErr:   ErrHostMemoryExhausted,
		},
		{
			name: "the running-container cap is enforced before memory",
			instances: []Instance{
				{Name: "alpha", Running: true, Memory: "512MiB"},
				{Name: "beta", Running: true, Memory: "512MiB"},
			},
			candidate:  "512MiB",
			container:  "gamma",
			maxRunning: 2,
			wantErr:    ErrTooManyRunning,
		},
		{
			name: "the running-container cap ignores stopped instances",
			instances: []Instance{
				{Name: "alpha", Running: true, Memory: "512MiB"},
				{Name: "beta", Running: false, Memory: "512MiB"},
			},
			candidate:  "512MiB",
			container:  "gamma",
			maxRunning: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fleet := &stubFleet{available: true, instances: test.instances}
			service := policyService(fleet, test.maxRunning)

			err := service.AuthorizeStart(context.Background(), test.container, test.candidate, test.force)
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("AuthorizeStart: %v", err)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("AuthorizeStart error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestAuthorizeStartFailsOpen(t *testing.T) {
	unavailable := policyService(&stubFleet{available: false}, 1)
	if err := unavailable.AuthorizeStart(context.Background(), "alpha", "8GiB", false); err != nil {
		t.Fatalf("an unreachable runtime must not block starts: %v", err)
	}

	blind := policyService(&stubFleet{available: true, listErr: errors.New("lxd is restarting")}, 1)
	if err := blind.AuthorizeStart(context.Background(), "alpha", "8GiB", false); err != nil {
		t.Fatalf("an unreadable fleet must not block starts: %v", err)
	}
}

func TestValidateOverride(t *testing.T) {
	service := policyService(&stubFleet{available: true}, 0)
	ctx := context.Background()

	tests := []struct {
		name    string
		memory  string
		cpu     float64
		disk    string
		wantErr error
	}{
		{name: "empty override inherits the defaults"},
		{name: "inside the ceiling", memory: "3GiB", cpu: 4, disk: "40GiB"},
		{name: "memory above the ceiling", memory: "4GiB", wantErr: ErrOverrideTooLarge},
		{name: "cpu above the ceiling", cpu: 8, wantErr: ErrOverrideTooLarge},
		{name: "disk above the ceiling", disk: "80GiB", wantErr: ErrOverrideTooLarge},
		{name: "unparsable memory", memory: "huge", wantErr: ErrInvalidSettings},
		{name: "unparsable disk", disk: "huge", wantErr: ErrInvalidSettings},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := service.ValidateOverride(ctx, test.memory, test.cpu, test.disk)
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateOverride: %v", err)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ValidateOverride error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateOverrideRejectsMoreThanTheHostHas(t *testing.T) {
	repo := &memoryRepository{found: true, settings: Settings{
		Defaults:           Limits{Memory: "2GiB", CPU: 1, Processes: 2000},
		HostReserve:        DefaultReserve(),
		MaxProjectOverride: Limits{Memory: "64GiB", CPU: 32},
	}}
	service := New(repo, hostOf(4*gib, 1), &stubFleet{available: true})

	if err := service.ValidateOverride(context.Background(), "8GiB", 0, ""); !errors.Is(err, ErrOverrideTooLarge) {
		t.Fatalf("memory above host capacity = %v, want ErrOverrideTooLarge", err)
	}
	if err := service.ValidateOverride(context.Background(), "", 4, ""); !errors.Is(err, ErrOverrideTooLarge) {
		t.Fatalf("cpu above host capacity = %v, want ErrOverrideTooLarge", err)
	}
}

func TestGetReportsHostArithmeticAndPoolCapability(t *testing.T) {
	fleet := &stubFleet{
		available: true,
		instances: []Instance{
			{Name: "alpha", Running: true, Memory: "2GiB"},
			{Name: "beta", Running: false, Memory: "2GiB"},
		},
		pool: PoolCapability{Pool: "default", Driver: "dir", Detail: "unsupported"},
	}
	view := policyService(fleet, 0).Get(context.Background())

	if view.Host.BudgetMemoryBytes != 8*gib-768*mib {
		t.Fatalf("budget = %d, want %d", view.Host.BudgetMemoryBytes, 8*gib-768*mib)
	}
	if view.Host.CommittedBytes != 2*gib || view.Host.RunningContainers != 1 {
		t.Fatalf("commitment = %d bytes over %d containers, want 2GiB over 1", view.Host.CommittedBytes, view.Host.RunningContainers)
	}
	if view.DiskQuota.Supported || view.DiskQuota.Driver != "dir" {
		t.Fatalf("disk quota = %+v, want an unsupported dir pool", view.DiskQuota)
	}
	if !view.Available {
		t.Fatal("runtime availability must be reported")
	}
}

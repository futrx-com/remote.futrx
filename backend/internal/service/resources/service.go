package resources

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Repository persists the policy document. Missing is not an error: the
// service derives a host-aware document on the first start that finds none.
type Repository interface {
	Load(ctx context.Context) (Settings, bool, error)
	Save(ctx context.Context, settings Settings) error
}

// HostProbe reports current host capacity. Backed by the same collector that
// serves `/api/server/info`, so the numbers an admin reads in Settings and the
// numbers the guard enforces are the same numbers.
type HostProbe interface {
	Facts(ctx context.Context) HostFacts
}

// Fleet is the container-runtime side of the policy: converging the managed
// profile, reporting whether the storage pool can enforce quotas, and listing
// what is running right now.
type Fleet interface {
	Available() bool
	ApplyDefaults(ctx context.Context, defaults Limits) error
	PoolCapability(ctx context.Context) (PoolCapability, error)
	RunningInstances(ctx context.Context) ([]Instance, error)
}

const (
	hostFactsTTL = 30 * time.Second
	poolTTL      = 5 * time.Minute
)

// Service is the single owner of the fleet resource policy.
type Service struct {
	repo  Repository
	host  HostProbe
	fleet Fleet

	mu       sync.RWMutex
	settings Settings
	loaded   bool

	facts   HostFacts
	factsAt time.Time

	pool   PoolCapability
	poolAt time.Time
}

func New(repo Repository, host HostProbe, fleet Fleet) *Service {
	return &Service{repo: repo, host: host, fleet: fleet}
}

// Ensure loads the policy - deriving and persisting it on first run - and
// converges the managed LXD profile onto it. Called once at startup and again
// after every admin change.
func (s *Service) Ensure(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}
	settings, found, err := s.repo.Load(ctx)
	if err != nil {
		return err
	}
	if !found {
		settings = DeriveSettings(s.hostFacts(ctx))
		settings.UpdatedAt = time.Now().Unix()
		if err := s.repo.Save(ctx, settings); err != nil {
			return err
		}
		log.Printf(
			"resources: derived first-run fleet defaults memory=%s cpu=%g disk=%s from host capacity",
			settings.Defaults.Memory, settings.Defaults.CPU, settings.Defaults.Disk,
		)
	}
	settings = normalize(settings)
	s.mu.Lock()
	s.settings = settings
	s.loaded = true
	s.mu.Unlock()
	return s.converge(ctx, settings)
}

func (s *Service) converge(ctx context.Context, settings Settings) error {
	if s.fleet == nil || !s.fleet.Available() {
		return nil
	}
	return s.fleet.ApplyDefaults(ctx, settings.Defaults)
}

// Settings returns the current policy, loading it lazily when Ensure has not
// run (or could not reach LXD) so read paths never observe a zero envelope.
func (s *Service) Settings(ctx context.Context) Settings {
	if s == nil {
		return normalize(Settings{})
	}
	s.mu.RLock()
	loaded, settings := s.loaded, s.settings
	s.mu.RUnlock()
	if loaded {
		return settings
	}
	stored, found := Settings{}, false
	if s.repo != nil {
		loaded, ok, err := s.repo.Load(ctx)
		stored, found = loaded, ok && err == nil
	}
	if !found {
		stored = DeriveSettings(s.hostFacts(ctx))
	}
	stored = normalize(stored)
	s.mu.Lock()
	s.settings, s.loaded = stored, true
	s.mu.Unlock()
	return stored
}

// Get assembles the complete admin view: policy, live host arithmetic, and
// storage-pool quota capability.
func (s *Service) Get(ctx context.Context) View {
	if s == nil {
		return View{Settings: normalize(Settings{})}
	}
	settings := s.Settings(ctx)
	return View{
		Settings:  settings,
		Host:      s.capacity(ctx, settings),
		DiskQuota: s.poolCapability(ctx),
		Available: s.fleet != nil && s.fleet.Available(),
	}
}

// Update validates, persists, and converges an operator change. Fields the
// caller left empty fall back to the current value, so a partial form submit
// cannot silently erase the reserve.
func (s *Service) Update(ctx context.Context, in Settings, actor string) (View, error) {
	if s == nil || s.repo == nil {
		return View{}, ErrUnavailable
	}
	current := s.Settings(ctx)
	merged := merge(current, in)
	if err := Validate(merged, s.hostFacts(ctx)); err != nil {
		return View{}, err
	}
	merged.Derived = false
	merged.UpdatedAt = time.Now().Unix()
	merged.UpdatedBy = strings.ToLower(strings.TrimSpace(actor))
	if err := s.repo.Save(ctx, merged); err != nil {
		return View{}, err
	}
	s.mu.Lock()
	s.settings, s.loaded = merged, true
	s.mu.Unlock()
	if err := s.converge(ctx, merged); err != nil {
		// The policy is persisted and every future launch will use it; only
		// the live profile write failed. Report it without losing the change.
		return s.Get(ctx), fmt.Errorf("apply fleet defaults: %w", err)
	}
	return s.Get(ctx), nil
}

// ValidateOverride rejects a per-project override that exceeds the operator's
// ceiling or the host's physical capacity.
func (s *Service) ValidateOverride(ctx context.Context, memory string, cpu float64, disk string) error {
	if s == nil {
		return nil
	}
	settings := s.Settings(ctx)
	facts := s.hostFacts(ctx)
	ceiling := settings.MaxProjectOverride

	if memory != "" {
		want, err := ParseSize(memory)
		if err != nil {
			return err
		}
		if limit, _ := ParseSize(ceiling.Memory); limit > 0 && want > limit {
			return fmt.Errorf("%w: memory %s is above the %s maximum", ErrOverrideTooLarge, memory, ceiling.Memory)
		}
		if facts.MemoryBytes > 0 && want > facts.MemoryBytes {
			return fmt.Errorf("%w: memory %s is above the host's %s", ErrOverrideTooLarge, memory, FormatSize(facts.MemoryBytes))
		}
	}
	if cpu > 0 {
		if ceiling.CPU > 0 && cpu > ceiling.CPU {
			return fmt.Errorf("%w: %g CPUs is above the %g maximum", ErrOverrideTooLarge, cpu, ceiling.CPU)
		}
		if facts.CPUs > 0 && cpu > float64(facts.CPUs) {
			return fmt.Errorf("%w: %g CPUs is above the host's %d", ErrOverrideTooLarge, cpu, facts.CPUs)
		}
	}
	if disk != "" {
		want, err := ParseSize(disk)
		if err != nil {
			return err
		}
		if limit, _ := ParseSize(ceiling.Disk); limit > 0 && want > limit {
			return fmt.Errorf("%w: disk %s is above the %s maximum", ErrOverrideTooLarge, disk, ceiling.Disk)
		}
	}
	return nil
}

// AuthorizeStart is the aggregate guard. It sums the memory ceilings of every
// RUNNING container plus the one about to start and refuses when the total
// would exceed host memory minus the reserve. Without it a host reboot
// autostarts every workspace at once and the box OOMs before anyone can log
// in. An admin can override with force.
func (s *Service) AuthorizeStart(ctx context.Context, containerName, memory string, force bool) error {
	if force || s == nil {
		return nil
	}
	if s.fleet == nil || !s.fleet.Available() {
		return nil
	}
	settings := s.Settings(ctx)
	instances, err := s.fleet.RunningInstances(ctx)
	if err != nil {
		// A guard that cannot see the fleet must not block work; the
		// per-container ceiling still applies.
		log.Printf("resources: aggregate guard skipped: %v", err)
		return nil
	}

	committed, running := commitment(instances, containerName, settings.Defaults.Memory)
	if settings.MaxRunningContainers > 0 && running >= settings.MaxRunningContainers {
		return fmt.Errorf(
			"%w: %d of %d containers already running - stop one first, or retry with force",
			ErrTooManyRunning, running, settings.MaxRunningContainers,
		)
	}

	facts := s.hostFacts(ctx)
	budget := memoryBudget(facts, settings.HostReserve)
	if budget == 0 {
		return nil
	}
	want, err := ParseSize(memory)
	if err != nil {
		return err
	}
	if want == 0 {
		want, _ = ParseSize(settings.Defaults.Memory)
	}
	if committed+want <= budget {
		return nil
	}
	return fmt.Errorf(
		"%w: %d running container(s) already commit %s of the %s available to workspaces, and this one asks for %s - stop another workspace, lower its memory limit, or retry with force",
		ErrHostMemoryExhausted, running, FormatSize(committed), FormatSize(budget), FormatSize(want),
	)
}

// commitment sums the memory ceilings of running containers, skipping the
// candidate itself (a restart must not count twice) and substituting the fleet
// default for an instance that carries no explicit ceiling.
func commitment(instances []Instance, skip, fallback string) (uint64, int) {
	fallbackBytes, _ := ParseSize(fallback)
	total := uint64(0)
	running := 0
	for _, instance := range instances {
		if !instance.Running || instance.Name == skip {
			continue
		}
		running++
		bytes, err := ParseSize(instance.Memory)
		if err != nil || bytes == 0 {
			bytes = fallbackBytes
		}
		total += bytes
	}
	return total, running
}

func memoryBudget(facts HostFacts, reserve Reserve) uint64 {
	reserveBytes, _ := ParseSize(reserve.Memory)
	if facts.MemoryBytes <= reserveBytes {
		return 0
	}
	return facts.MemoryBytes - reserveBytes
}

func (s *Service) capacity(ctx context.Context, settings Settings) HostCapacity {
	facts := s.hostFacts(ctx)
	capacity := HostCapacity{
		MemoryBytes:       facts.MemoryBytes,
		CPUs:              facts.CPUs,
		DiskBytes:         facts.DiskBytes,
		BudgetMemoryBytes: memoryBudget(facts, settings.HostReserve),
	}
	capacity.ReserveMemoryBytes, _ = ParseSize(settings.HostReserve.Memory)
	if s.fleet != nil && s.fleet.Available() {
		if instances, err := s.fleet.RunningInstances(ctx); err == nil {
			capacity.CommittedBytes, capacity.RunningContainers = commitment(instances, "", settings.Defaults.Memory)
		}
	}
	return capacity
}

func (s *Service) hostFacts(ctx context.Context) HostFacts {
	s.mu.RLock()
	facts, at := s.facts, s.factsAt
	s.mu.RUnlock()
	if !at.IsZero() && time.Since(at) < hostFactsTTL {
		return facts
	}
	if s.host == nil {
		return HostFacts{}
	}
	fresh := s.host.Facts(ctx)
	s.mu.Lock()
	s.facts, s.factsAt = fresh, time.Now()
	s.mu.Unlock()
	return fresh
}

func (s *Service) poolCapability(ctx context.Context) PoolCapability {
	s.mu.RLock()
	pool, at := s.pool, s.poolAt
	s.mu.RUnlock()
	if !at.IsZero() && time.Since(at) < poolTTL {
		return pool
	}
	if s.fleet == nil || !s.fleet.Available() {
		return PoolCapability{Detail: "container runtime unavailable"}
	}
	fresh, err := s.fleet.PoolCapability(ctx)
	if err != nil {
		return PoolCapability{Detail: err.Error()}
	}
	s.mu.Lock()
	s.pool, s.poolAt = fresh, time.Now()
	s.mu.Unlock()
	return fresh
}

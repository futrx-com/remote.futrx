package project

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	repo               Repository
	containerLifecycle ContainerLifecycle
	containerInspector ContainerInspector
	containerNetwork   ContainerNetwork
	containerListeners ContainerListeners

	// access owns per-project membership and email normalization.
	access *accessList

	// secrets owns the project's environment variables and their propagation
	// to the workspace .env file and the container environment.
	secrets *secretStore

	// browsers owns all Agent Browser state and its idle reaper.
	browsers *agentBrowsers

	// runState serializes container run-state transitions per project so a
	// container deleted out-of-band (e.g. a workspace recycle onto a new base
	// image) is relaunched exactly once even under concurrent prompts. It also
	// keeps an explicit stop, restart, or delete from racing an agent-triggered
	// recovery. Different projects remain independent.
	runState keyedMutex
}

func New(
	repo Repository,
	containers ContainerDependencies,
	secrets SecretsRepository,
	access AccessRepository,
) *Service {
	return &Service{
		repo:               repo,
		containerLifecycle: containers.Lifecycle,
		containerInspector: containers.Inspector,
		containerNetwork:   containers.Network,
		containerListeners: containers.Listeners,
		access:             newAccessList(access),
		secrets:            newSecretStore(secrets, containers.Environment),
		browsers:           newAgentBrowsers(containers.Browser, repo),
	}
}

func (s *Service) ListSecrets(ctx context.Context, id ID) ([]Secret, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.secrets.list(ctx, id)
}

func (s *Service) SetSecret(ctx context.Context, id ID, key, value string) (Secret, error) {
	if !ValidID(id) {
		return Secret{}, ErrInvalidID
	}
	if !ValidSecretKey(key) {
		return Secret{}, ErrInvalidSecretKey
	}
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return Secret{}, err
	}
	return s.secrets.set(ctx, m, key, value)
}

func (s *Service) DeleteSecret(ctx context.Context, id ID, key string) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	if !ValidSecretKey(key) {
		return ErrInvalidSecretKey
	}
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	return s.secrets.delete(ctx, m, key)
}

// List returns every project. Use ListVisible to apply per-caller filtering.
func (s *Service) List(ctx context.Context) ([]Meta, error) {
	return s.repo.List(ctx)
}

// ListVisible returns projects the caller can see. Admins see everything;
// non-admins only see projects where they're a member. callerEmail is
// already normalized when this is called from the handler.
func (s *Service) ListVisible(ctx context.Context, callerEmail string, isAdmin bool) ([]Meta, error) {
	all, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if isAdmin {
		return all, nil
	}
	return s.access.filterVisible(ctx, all, callerEmail), nil
}

func (s *Service) Get(ctx context.Context, id ID) (Meta, error) {
	if !ValidID(id) {
		return Meta{}, ErrInvalidID
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (Meta, error) {
	if !ValidSlug(slug) {
		return Meta{}, ErrNotFound
	}
	return s.repo.GetBySlug(ctx, slug)
}

func (s *Service) WorkspaceForProject(ctx context.Context, id ID) (string, error) {
	p, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return p.Cwd, nil
}

// Create provisions a new project. callerEmail (if non-empty) is added to
// the project's access list so the creator immediately has access.
func (s *Service) Create(ctx context.Context, in CreateInput, callerEmail string) (Meta, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Meta{}, ErrNameRequired
	}

	m, err := s.repo.Create(ctx, Meta{
		Name:   name,
		Slug:   Slugify(name),
		Status: StatusProvisioning,
	})
	if err != nil {
		return Meta{}, err
	}

	s.access.seed(ctx, m.ID, callerEmail)

	if s.containerLifecycle != nil {
		if err := s.containerLifecycle.Ensure(ctx, m); err != nil {
			log.Printf("projects: ensure %s failed: %v", m.ContainerName, err)
			return s.repo.SetStatus(ctx, m.ID, StatusError, err.Error())
		}
		// Push any pre-existing project secrets into the freshly launched
		// container's env. Empty on first create; matters on recreate
		// (delete + relaunch with secrets already stored).
		if syncErr := s.secrets.syncContainer(ctx, m.ID, m.ContainerName); syncErr != nil {
			log.Printf("projects: sync env to %s after launch: %v", m.ContainerName, syncErr)
		}
	}
	return s.repo.SetStatus(ctx, m.ID, StatusRunning, "")
}

func (s *Service) Update(ctx context.Context, id ID, in UpdateInput) (Meta, error) {
	if !ValidID(id) {
		return Meta{}, ErrInvalidID
	}
	return s.repo.Update(ctx, id, func(m *Meta) {
		if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
			m.Name = strings.TrimSpace(*in.Name)
		}
	})
}

var resourceSizePattern = regexp.MustCompile(`^[1-9][0-9]*(MiB|GiB|TiB)$`)

// SetContainerLimits validates and persists project-level LXD overrides. The
// running or stopped container is updated immediately; a missing container
// retains the desired values in metadata for its next launch.
func (s *Service) SetContainerLimits(ctx context.Context, id ID, limits ContainerLimits) (ContainerInspect, error) {
	if !ValidID(id) {
		return ContainerInspect{}, ErrInvalidID
	}
	normalized, err := normalizeContainerLimits(limits)
	if err != nil {
		return ContainerInspect{}, err
	}
	meta, err := s.repo.Get(ctx, id)
	if err != nil {
		return ContainerInspect{}, err
	}

	if s.containerLifecycle != nil && meta.ContainerName != "" {
		state, stateErr := s.containerLifecycle.State(ctx, meta.ContainerName)
		if stateErr != nil {
			return ContainerInspect{}, stateErr
		}
		if state != ContainerStateMissing {
			if err := s.containerLifecycle.SetResourceLimits(ctx, meta.ContainerName, normalized); err != nil {
				return ContainerInspect{}, err
			}
		}
	}

	meta, err = s.repo.Update(ctx, id, func(project *Meta) {
		if limitsEmpty(normalized) {
			project.ResourceLimits = nil
			return
		}
		copy := normalized
		project.ResourceLimits = &copy
	})
	if err != nil {
		return ContainerInspect{}, err
	}
	if s.containerInspector == nil || meta.ContainerName == "" {
		return ContainerInspect{Name: meta.ContainerName, LimitOverrides: meta.ResourceLimits}, nil
	}
	info, err := s.containerInspector.Inspect(ctx, meta.ContainerName)
	if err != nil {
		// The mutation and persistence already succeeded. Keep the response
		// successful even when the follow-up best-effort snapshot is unavailable.
		return ContainerInspect{Name: meta.ContainerName, LimitOverrides: meta.ResourceLimits}, nil
	}
	info.LimitOverrides = meta.ResourceLimits
	return info, nil
}

func normalizeContainerLimits(limits ContainerLimits) (ContainerLimits, error) {
	limits.CPU = strings.TrimSpace(limits.CPU)
	limits.Memory = strings.TrimSpace(limits.Memory)
	limits.Disk = strings.TrimSpace(limits.Disk)
	if limits.CPU != "" {
		cores, err := strconv.Atoi(limits.CPU)
		if err != nil || cores < 1 || cores > 256 {
			return ContainerLimits{}, ErrInvalidLimits
		}
	}
	if limits.Memory != "" && !resourceSizePattern.MatchString(limits.Memory) {
		return ContainerLimits{}, ErrInvalidLimits
	}
	if limits.Disk != "" && !resourceSizePattern.MatchString(limits.Disk) {
		return ContainerLimits{}, ErrInvalidLimits
	}
	return limits, nil
}

func limitsEmpty(limits ContainerLimits) bool {
	return limits.CPU == "" && limits.Memory == "" && limits.Disk == ""
}

func (s *Service) Reorder(ctx context.Context, ids []ID) ([]Meta, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	base := time.Now().UnixMilli()
	seen := map[ID]bool{}
	out := make([]Meta, 0, len(ids))
	for i, id := range ids {
		if !ValidID(id) {
			return nil, ErrInvalidID
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		order := base - int64(i)
		m, err := s.repo.Update(ctx, id, func(m *Meta) {
			m.Order = order
		})
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *Service) Delete(ctx context.Context, id ID) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	unlock := s.runState.lock(id)
	defer unlock()
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	s.browsers.clearState(id)
	if s.containerLifecycle != nil && m.ContainerName != "" {
		if err := s.containerLifecycle.Delete(ctx, m.ContainerName); err != nil {
			log.Printf("projects: delete container %s: %v", m.ContainerName, err)
		}
	}
	if err := s.secrets.deleteAll(ctx, id); err != nil {
		log.Printf("projects: delete secrets %s: %v", id, err)
	}
	if err := s.access.deleteAll(ctx, id); err != nil {
		log.Printf("projects: delete access %s: %v", id, err)
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) Start(ctx context.Context, id ID) (Meta, error) {
	if !ValidID(id) {
		return Meta{}, ErrInvalidID
	}
	unlock := s.runState.lock(id)
	defer unlock()
	return s.startLocked(ctx, id)
}

// startLocked converges one project to RUNNING while the caller owns its
// run-state lock. Restart uses this directly for a missing instance to avoid
// recursively acquiring the same non-reentrant mutex.
func (s *Service) startLocked(ctx context.Context, id ID) (Meta, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return Meta{}, err
	}
	if s.containerLifecycle != nil {
		state, err := s.containerLifecycle.State(ctx, m.ContainerName)
		if err != nil {
			return s.setStartError(ctx, id, err)
		}
		if err := s.containerLifecycle.Ensure(ctx, m); err != nil {
			return s.setStartError(ctx, id, err)
		}
		if state == ContainerStateMissing {
			if syncErr := s.secrets.syncContainer(ctx, id, m.ContainerName); syncErr != nil {
				log.Printf("projects: sync env to %s after ensure: %v", m.ContainerName, syncErr)
			}
		}
	}
	return s.repo.SetStatus(ctx, id, StatusRunning, "")
}

var ErrProjectBusy = errors.New("project has an active agent process")

// Upgrade replaces one project container through the same convergence path
// used by normal starts. Legacy agent homes are migrated before deletion.
func (s *Service) Upgrade(ctx context.Context, id ID, includeBusy bool) (Meta, error) {
	if !ValidID(id) {
		return Meta{}, ErrInvalidID
	}
	unlock := s.runState.lock(id)
	defer unlock()
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return Meta{}, err
	}
	if s.containerLifecycle == nil {
		return m, nil
	}
	state, err := s.containerLifecycle.State(ctx, m.ContainerName)
	if err != nil {
		return s.setStartError(ctx, id, err)
	}
	if state != ContainerStateMissing {
		busy, err := s.containerLifecycle.Busy(ctx, m.ContainerName)
		if err != nil {
			return s.setStartError(ctx, id, err)
		}
		if busy && !includeBusy {
			return m, ErrProjectBusy
		}
		if busy {
			if err := s.containerLifecycle.Stop(ctx, m.ContainerName); err != nil {
				return s.setStartError(ctx, id, err)
			}
		}
		if err := s.containerLifecycle.Ensure(ctx, m); err != nil {
			return s.setStartError(ctx, id, err)
		}
		s.browsers.stopBeforeUpgrade(ctx, id, m.ContainerName)
		if err := s.containerLifecycle.Delete(ctx, m.ContainerName); err != nil {
			return s.setStartError(ctx, id, err)
		}
	}
	if err := s.containerLifecycle.Ensure(ctx, m); err != nil {
		return s.setStartError(ctx, id, err)
	}
	if syncErr := s.secrets.syncContainer(ctx, id, m.ContainerName); syncErr != nil {
		log.Printf("projects: sync env to %s after upgrade: %v", m.ContainerName, syncErr)
	}
	return s.repo.SetStatus(ctx, id, StatusRunning, "")
}

// setStartError keeps the persisted project status useful for the UI while
// returning the operational failure to callers. Agent providers must stop here
// instead of attempting CLI provisioning against a container that never
// started and masking the real cause with a later "instance not found" error.
func (s *Service) setStartError(ctx context.Context, id ID, cause error) (Meta, error) {
	m, statusErr := s.repo.SetStatus(ctx, id, StatusError, cause.Error())
	if statusErr != nil {
		return Meta{}, errors.Join(cause, statusErr)
	}
	return m, cause
}

func (s *Service) Stop(ctx context.Context, id ID) (Meta, error) {
	if !ValidID(id) {
		return Meta{}, ErrInvalidID
	}
	unlock := s.runState.lock(id)
	defer unlock()
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return Meta{}, err
	}
	if s.containerLifecycle != nil {
		if err := s.containerLifecycle.Stop(ctx, m.ContainerName); err != nil {
			return s.repo.SetStatus(ctx, id, StatusError, err.Error())
		}
	}
	return s.repo.SetStatus(ctx, id, StatusStopped, "")
}

// Restart force-restarts the project's container. Unlike Stop, this works
// even when the workspace is wedged at its resource limits: the kill comes
// from the host kernel and needs no cooperation from processes inside. A
// missing container is launched instead, so Restart always converges on a
// running workspace.
func (s *Service) Restart(ctx context.Context, id ID) (Meta, error) {
	if !ValidID(id) {
		return Meta{}, ErrInvalidID
	}
	unlock := s.runState.lock(id)
	defer unlock()
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return Meta{}, err
	}
	if s.containerLifecycle != nil {
		state, err := s.containerLifecycle.State(ctx, m.ContainerName)
		if err != nil {
			return s.repo.SetStatus(ctx, id, StatusError, err.Error())
		}
		if state == ContainerStateMissing {
			return s.startLocked(ctx, id)
		}
		if err := s.containerLifecycle.Restart(ctx, m.ContainerName); err != nil {
			return s.repo.SetStatus(ctx, id, StatusError, err.Error())
		}
	}
	return s.repo.SetStatus(ctx, id, StatusRunning, "")
}

func (s *Service) InspectContainer(ctx context.Context, id ID) (ContainerInspect, error) {
	if !ValidID(id) {
		return ContainerInspect{}, ErrInvalidID
	}
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return ContainerInspect{}, err
	}
	if s.containerInspector == nil || m.ContainerName == "" {
		return ContainerInspect{Name: m.ContainerName, LimitOverrides: m.ResourceLimits}, nil
	}
	info, err := s.containerInspector.Inspect(ctx, m.ContainerName)
	if err != nil {
		return ContainerInspect{}, err
	}
	info.LimitOverrides = m.ResourceLimits
	return info, nil
}

// RepairNetwork re-runs eth0 configuration inside the project's container
// and returns a fresh inspection once an IPv4 address is back (or after a
// short grace period if DHCP is slow). Manual recovery for the
// networkd-dropped-lease failure mode.
func (s *Service) RepairNetwork(ctx context.Context, id ID) (ContainerInspect, error) {
	if !ValidID(id) {
		return ContainerInspect{}, ErrInvalidID
	}
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return ContainerInspect{}, err
	}
	if s.containerNetwork == nil || s.containerInspector == nil || m.ContainerName == "" {
		return ContainerInspect{Name: m.ContainerName, LimitOverrides: m.ResourceLimits}, nil
	}
	if err := s.containerNetwork.Repair(ctx, m.ContainerName); err != nil {
		return ContainerInspect{}, err
	}
	info, err := s.containerInspector.Inspect(ctx, m.ContainerName)
	for i := 0; i < 5 && err == nil && !hasIPv4(info); i++ {
		select {
		case <-ctx.Done():
			info.LimitOverrides = m.ResourceLimits
			return info, err
		case <-time.After(time.Second):
		}
		info, err = s.containerInspector.Inspect(ctx, m.ContainerName)
	}
	info.LimitOverrides = m.ResourceLimits
	return info, err
}

func hasIPv4(info ContainerInspect) bool {
	for _, n := range info.Network {
		for _, a := range n.Addresses {
			if strings.Count(a, ".") == 3 {
				return true
			}
		}
	}
	return false
}

func (s *Service) ListContainerApps(ctx context.Context, id ID) ([]ContainerApp, error) {
	if !ValidID(id) {
		return nil, ErrInvalidID
	}
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.containerListeners == nil || m.ContainerName == "" {
		return nil, nil
	}
	return s.containerListeners.List(ctx, m.ContainerName)
}

// TouchAgentBrowserActivity records a browser-use heartbeat for idle reaping.
func (s *Service) TouchAgentBrowserActivity(ctx context.Context, id ID) {
	_ = ctx
	s.browsers.touch(id)
}

// StartAgentBrowser ensures the project's container is running, records the
// Agent Browser as starting, and provisions the stack in the background.
// Idempotent while a start is already in flight.
func (s *Service) StartAgentBrowser(ctx context.Context, id ID) (AgentBrowserInfo, error) {
	m, err := s.Start(ctx, id)
	if err != nil {
		return AgentBrowserInfo{}, err
	}
	return s.browsers.start(ctx, id, m)
}

// AgentBrowserStatus returns split core/view browser state and records a
// heartbeat so an open pane keeps the idle reaper from tearing down the stack.
func (s *Service) AgentBrowserStatus(ctx context.Context, id ID) (AgentBrowserInfo, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return AgentBrowserInfo{}, err
	}
	return s.browsers.status(ctx, id, m)
}

// StopAgentBrowser tears down the Agent Browser stack in the project's
// container, leaving the container running and the persistent browser
// profile on disk so logins survive.
func (s *Service) StopAgentBrowser(ctx context.Context, id ID) error {
	m, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	return s.browsers.stop(ctx, id, m)
}

// StopAgentBrowserView tears down only the human noVNC layer.
func (s *Service) StopAgentBrowserView(ctx context.Context, id ID) error {
	m, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	return s.browsers.stopView(ctx, m)
}

// StartAgentBrowserReaper stops browser stacks that have had no agent or pane
// activity for ttl. It is safe to call multiple times; only the first call
// starts a ticker.
func (s *Service) StartAgentBrowserReaper(ctx context.Context, ttl time.Duration) {
	s.browsers.startReaper(ctx, ttl)
}

func (s *Service) Reconcile(ctx context.Context) error {
	if s.containerLifecycle == nil || !s.containerLifecycle.Available() {
		return nil
	}
	metas, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	converged := 0
	for _, m := range metas {
		state, err := s.containerLifecycle.State(ctx, m.ContainerName)
		if err != nil {
			continue
		}
		want := statusForContainerState(state)
		if want != m.Status {
			if _, err := s.repo.SetStatus(ctx, m.ID, want, ""); err != nil {
				log.Printf("projects: reconcile %s: %v", m.ID, err)
			}
		}
		// Converge the resource envelope on every existing container so a
		// deploy (backend restart) alone brings the whole fleet to the
		// pinned limits — no per-project action needed on any box.
		if state != ContainerStateMissing {
			if err := s.containerLifecycle.EnsureResources(ctx, m.ContainerName); err != nil {
				log.Printf("projects: reconcile resources %s/%s: %v", m.ID, m.ContainerName, err)
			} else {
				converged++
			}
			if m.ResourceLimits != nil {
				if err := s.containerLifecycle.SetResourceLimits(ctx, m.ContainerName, *m.ResourceLimits); err != nil {
					log.Printf("projects: reconcile resource overrides %s/%s: %v", m.ID, m.ContainerName, err)
				}
			}
		}
	}
	if converged > 0 {
		log.Printf("projects: resource envelope converged on %d container(s)", converged)
	}
	return nil
}

// HasAccess returns true if email is in the project's membership list.
// Empty / unknown email always returns false. Admin checks live in the
// caller; this method only looks at the access list.
func (s *Service) HasAccess(ctx context.Context, id ID, email string) (bool, error) {
	if !ValidID(id) {
		return false, ErrInvalidID
	}
	return s.access.has(ctx, id, email)
}

// ListAccess returns the sorted, normalized membership list for a project.
func (s *Service) ListAccess(ctx context.Context, id ID) ([]string, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.access.list(ctx, id)
}

// AddAccess adds email to the project's membership list. Caller is
// responsible for verifying the email belongs to a registered user.
func (s *Service) AddAccess(ctx context.Context, id ID, email string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return s.access.add(ctx, id, email)
}

// RemoveAccess deletes email from the project's membership list.
func (s *Service) RemoveAccess(ctx context.Context, id ID, email string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return s.access.remove(ctx, id, email)
}

// CountAccess returns how many members the project has.
func (s *Service) CountAccess(ctx context.Context, id ID) (int, error) {
	if !ValidID(id) {
		return 0, ErrInvalidID
	}
	return s.access.count(ctx, id)
}

func statusForContainerState(state ContainerState) Status {
	switch state {
	case ContainerStateRunning:
		return StatusRunning
	case ContainerStateStopped:
		return StatusStopped
	case ContainerStateMissing:
		return StatusMissing
	default:
		return StatusUnknown
	}
}

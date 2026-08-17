package project

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Service struct {
	repo                 Repository
	containerLifecycle   ContainerLifecycle
	containerEnvironment ContainerEnvironment
	containerInspector   ContainerInspector
	containerNetwork     ContainerNetwork
	containerListeners   ContainerListeners
	containerBrowser     ContainerBrowser
	containerPolicy      ContainerPolicy
	containerAdmission   ContainerAdmission
	secrets              SecretsRepository
	access               AccessRepository

	agentBrowserMu    sync.Mutex
	agentBrowserInfo  map[ID]AgentBrowserInfo
	agentBrowserStart map[ID]int64

	browserActivityMu sync.Mutex
	browserSeen       map[ID]time.Time
	reaperOn          bool

	// runStateLocks serializes container run-state transitions per project so a
	// container deleted out-of-band (e.g. a workspace recycle onto a new base
	// image) is relaunched exactly once even under concurrent prompts.
	runStateMu    sync.Mutex
	runStateLocks map[ID]*sync.Mutex
}

func New(
	repo Repository,
	containers ContainerDependencies,
	secrets SecretsRepository,
	access AccessRepository,
) *Service {
	return &Service{
		repo:                 repo,
		containerLifecycle:   containers.Lifecycle,
		containerEnvironment: containers.Environment,
		containerInspector:   containers.Inspector,
		containerNetwork:     containers.Network,
		containerListeners:   containers.Listeners,
		containerBrowser:     containers.Browser,
		containerPolicy:      containers.Policy,
		containerAdmission:   containers.Admission,
		secrets:              secrets,
		access:               access,
		agentBrowserInfo:     make(map[ID]AgentBrowserInfo),
		agentBrowserStart:    make(map[ID]int64),
	}
}

func (s *Service) ListSecrets(ctx context.Context, id ID) ([]Secret, error) {
	if !ValidID(id) {
		return nil, ErrInvalidID
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	if s.secrets == nil {
		return nil, nil
	}
	return s.secrets.List(ctx, id)
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
	if s.secrets == nil {
		return Secret{}, ErrSecretsUnavailable
	}
	saved, err := s.secrets.Set(ctx, id, key, value)
	if err != nil {
		return Secret{}, err
	}
	if syncErr := s.syncEnvFile(ctx, id, m.Cwd); syncErr != nil {
		log.Printf("projects: sync .env for %s after set %s: %v", id, key, syncErr)
	}
	if s.containerEnvironment != nil && m.ContainerName != "" {
		if envErr := s.containerEnvironment.ApplyDiff(
			ctx, m.ContainerName,
			map[string]string{key: value}, nil,
		); envErr != nil {
			log.Printf("projects: push env %s to %s: %v", key, m.ContainerName, envErr)
		}
	}
	return saved, nil
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
	if s.secrets == nil {
		return ErrSecretsUnavailable
	}
	if err := s.secrets.Delete(ctx, id, key); err != nil {
		return err
	}
	if syncErr := s.syncEnvFile(ctx, id, m.Cwd); syncErr != nil {
		log.Printf("projects: sync .env for %s after delete %s: %v", id, key, syncErr)
	}
	if s.containerEnvironment != nil && m.ContainerName != "" {
		if envErr := s.containerEnvironment.ApplyDiff(
			ctx, m.ContainerName, nil, []string{key},
		); envErr != nil {
			log.Printf("projects: unset env %s on %s: %v", key, m.ContainerName, envErr)
		}
	}
	return nil
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
	if isAdmin || s.access == nil {
		return all, nil
	}
	callerEmail = strings.ToLower(strings.TrimSpace(callerEmail))
	if callerEmail == "" {
		return nil, nil
	}
	out := make([]Meta, 0, len(all))
	for _, m := range all {
		ok, err := s.access.Has(ctx, m.ID, callerEmail)
		if err != nil {
			log.Printf("projects: access check %s/%s: %v", m.ID, callerEmail, err)
			continue
		}
		if ok {
			out = append(out, m)
		}
	}
	return out, nil
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

	if s.access != nil {
		em := strings.ToLower(strings.TrimSpace(callerEmail))
		if em != "" {
			if addErr := s.access.Add(ctx, m.ID, em); addErr != nil {
				log.Printf("projects: seed access for %s: %v", m.ID, addErr)
			}
		}
	}

	if s.containerLifecycle != nil {
		if err := s.authorizeStart(ctx, m, false); err != nil {
			return s.repo.SetStatus(ctx, m.ID, StatusError, err.Error())
		}
		if err := s.containerLifecycle.Ensure(ctx, s.withEffectiveLimits(ctx, m)); err != nil {
			log.Printf("projects: ensure %s failed: %v", m.ContainerName, err)
			return s.repo.SetStatus(ctx, m.ID, StatusError, err.Error())
		}
		// Push any pre-existing project secrets into the freshly launched
		// container's env. Empty on first create; matters on recreate
		// (delete + relaunch with secrets already stored).
		if syncErr := s.syncContainerEnv(ctx, m.ID, m.ContainerName); syncErr != nil {
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

// normalizeContainerLimits trims and range-checks one per-project override.
// Grammar only: the fleet ceiling and host-capacity checks live in the
// resource policy, which this layer consults through ContainerPolicy.
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
	unlock := s.lockRunState(id)
	defer unlock()
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	s.clearAgentBrowserState(id)
	if s.containerLifecycle != nil && m.ContainerName != "" {
		if err := s.containerLifecycle.Delete(ctx, m.ContainerName); err != nil {
			log.Printf("projects: delete container %s: %v", m.ContainerName, err)
		}
	}
	if s.secrets != nil {
		if err := s.secrets.DeleteAll(ctx, id); err != nil {
			log.Printf("projects: delete secrets %s: %v", id, err)
		}
	}
	if s.access != nil {
		if err := s.access.DeleteAll(ctx, id); err != nil {
			log.Printf("projects: delete access %s: %v", id, err)
		}
	}
	return s.repo.Delete(ctx, id)
}

// lockRunState serializes container lifecycle transitions for a single project.
// This prevents double-launches and keeps an explicit stop, restart, or delete
// from racing an agent-triggered recovery. Different projects remain independent.
func (s *Service) lockRunState(id ID) func() {
	s.runStateMu.Lock()
	if s.runStateLocks == nil {
		s.runStateLocks = make(map[ID]*sync.Mutex)
	}
	mu := s.runStateLocks[id]
	if mu == nil {
		mu = &sync.Mutex{}
		s.runStateLocks[id] = mu
	}
	s.runStateMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

func (s *Service) Start(ctx context.Context, id ID) (Meta, error) {
	return s.StartWithOptions(ctx, id, StartOptions{})
}

// StartWithOptions converges one project to RUNNING. Force skips the
// aggregate admission guard; only an admin may set it.
func (s *Service) StartWithOptions(ctx context.Context, id ID, options StartOptions) (Meta, error) {
	if !ValidID(id) {
		return Meta{}, ErrInvalidID
	}
	unlock := s.lockRunState(id)
	defer unlock()
	return s.startLocked(ctx, id, options)
}

// startLocked converges one project to RUNNING while the caller owns its
// run-state lock. Restart uses this directly for a missing instance to avoid
// recursively acquiring the same non-reentrant mutex.
func (s *Service) startLocked(ctx context.Context, id ID, options StartOptions) (Meta, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return Meta{}, err
	}
	if s.containerLifecycle != nil {
		state, err := s.containerLifecycle.State(ctx, m.ContainerName)
		if err != nil {
			return s.setStartError(ctx, id, err)
		}
		// A container that is already running commits its memory whether or
		// not this call succeeds; only a real start needs admission.
		if state != ContainerStateRunning {
			if err := s.authorizeStart(ctx, m, options.Force); err != nil {
				return Meta{}, err
			}
		}
		if err := s.containerLifecycle.Ensure(ctx, s.withEffectiveLimits(ctx, m)); err != nil {
			return s.setStartError(ctx, id, err)
		}
		if state == ContainerStateMissing {
			if syncErr := s.syncContainerEnv(ctx, id, m.ContainerName); syncErr != nil {
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
	unlock := s.lockRunState(id)
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
		if err := s.containerLifecycle.Ensure(ctx, s.withEffectiveLimits(ctx, m)); err != nil {
			return s.setStartError(ctx, id, err)
		}
		s.stopAgentBrowserBeforeUpgrade(ctx, id, m.ContainerName)
		if err := s.containerLifecycle.Delete(ctx, m.ContainerName); err != nil {
			return s.setStartError(ctx, id, err)
		}
	}
	if err := s.containerLifecycle.Ensure(ctx, s.withEffectiveLimits(ctx, m)); err != nil {
		return s.setStartError(ctx, id, err)
	}
	if syncErr := s.syncContainerEnv(ctx, id, m.ContainerName); syncErr != nil {
		log.Printf("projects: sync env to %s after upgrade: %v", m.ContainerName, syncErr)
	}
	return s.repo.SetStatus(ctx, id, StatusRunning, "")
}

// Browser profiles are durable host mounts. Stop Chromium gracefully so it
// can remove its Singleton* locks before the disposable container is deleted.
// A missing or legacy browser stack must not block the workspace upgrade; the
// replacement container provisions it on demand.
func (s *Service) stopAgentBrowserBeforeUpgrade(ctx context.Context, id ID, containerName string) {
	if s.containerBrowser == nil {
		return
	}
	if err := s.containerBrowser.Stop(ctx, containerName); err != nil {
		log.Printf("projects: stop agent browser in %s before upgrade: %v", containerName, err)
	}
	s.clearAgentBrowserState(id)
	s.forgetAgentBrowserActivity(id)
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
	unlock := s.lockRunState(id)
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
	unlock := s.lockRunState(id)
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
			return s.startLocked(ctx, id, StartOptions{})
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
	if !ValidID(id) {
		return
	}
	s.browserActivityMu.Lock()
	defer s.browserActivityMu.Unlock()
	if s.browserSeen == nil {
		s.browserSeen = map[ID]time.Time{}
	}
	s.browserSeen[id] = time.Now()
}

func (s *Service) browserLastActivity(id ID) int64 {
	s.browserActivityMu.Lock()
	defer s.browserActivityMu.Unlock()
	if s.browserSeen == nil {
		return 0
	}
	t := s.browserSeen[id]
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func (s *Service) forgetAgentBrowserActivity(id ID) {
	s.browserActivityMu.Lock()
	defer s.browserActivityMu.Unlock()
	if s.browserSeen != nil {
		delete(s.browserSeen, id)
	}
}

// StartAgentBrowser ensures the project's container is running, records the
// Agent Browser as starting, and provisions the stack in the background.
// Idempotent while a start is already in flight.
func (s *Service) StartAgentBrowser(ctx context.Context, id ID) (AgentBrowserInfo, error) {
	m, err := s.Start(ctx, id)
	if err != nil {
		return AgentBrowserInfo{}, err
	}
	if s.containerBrowser == nil || m.ContainerName == "" {
		return AgentBrowserInfo{}, errors.New("project has no container to run the browser in")
	}
	s.TouchAgentBrowserActivity(ctx, id)
	if info, ok := s.agentBrowserState(id); ok && info.Status == AgentBrowserStatusStarting {
		return info, nil
	}
	current, err := s.containerBrowser.Status(ctx, m.ContainerName)
	if err != nil {
		return AgentBrowserInfo{}, err
	}
	if current.Status == AgentBrowserStatusReady {
		current.Slug = m.Slug
		current.Port = s.containerBrowser.Port()
		current.LastActivity = s.browserLastActivity(id)
		s.setAgentBrowserState(id, current)
		return current, nil
	}

	info, startID := s.beginAgentBrowserStart(id, m)
	go s.ensureAgentBrowserStarted(id, startID, m)
	return info, nil
}

// AgentBrowserStatus returns split core/view browser state and records a
// heartbeat so an open pane keeps the idle reaper from tearing down the stack.
func (s *Service) AgentBrowserStatus(ctx context.Context, id ID) (AgentBrowserInfo, error) {
	if !ValidID(id) {
		return AgentBrowserInfo{}, ErrInvalidID
	}
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return AgentBrowserInfo{}, err
	}
	s.TouchAgentBrowserActivity(ctx, id)
	info := s.agentBrowserInfoFor(m, AgentBrowserStatusStopped, "")
	if s.containerBrowser == nil || m.ContainerName == "" {
		info.LastActivity = s.browserLastActivity(id)
		return info, nil
	}
	stored, hasStored := s.agentBrowserState(id)
	containerInfo, err := s.containerBrowser.Status(ctx, m.ContainerName)
	if err != nil {
		return AgentBrowserInfo{}, err
	}
	containerInfo.Slug = m.Slug
	containerInfo.Port = s.containerBrowser.Port()
	containerInfo.LastActivity = s.browserLastActivity(id)
	if containerInfo.Core == "ready" {
		info = containerInfo
		if info.Status == AgentBrowserStatusCoreReady && info.View == "ready" {
			info.Status = AgentBrowserStatusReady
		}
		s.setAgentBrowserState(id, info)
		return info, nil
	}
	if hasStored {
		switch stored.Status {
		case AgentBrowserStatusStarting, AgentBrowserStatusError:
			stored.LastActivity = s.browserLastActivity(id)
			return stored, nil
		case AgentBrowserStatusReady:
			s.clearAgentBrowserState(id)
		}
	}
	info.Core = containerInfo.Core
	info.View = containerInfo.View
	info.ViewerCount = containerInfo.ViewerCount
	info.UptimeSec = containerInfo.UptimeSec
	info.LastActivity = s.browserLastActivity(id)
	return info, nil
}

// StopAgentBrowser tears down the Agent Browser stack in the project's
// container, leaving the container running and the persistent browser
// profile on disk so logins survive.
func (s *Service) StopAgentBrowser(ctx context.Context, id ID) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if s.containerBrowser == nil || m.ContainerName == "" {
		s.clearAgentBrowserState(id)
		s.forgetAgentBrowserActivity(id)
		return nil
	}
	s.clearAgentBrowserState(id)
	if err := s.containerBrowser.Stop(ctx, m.ContainerName); err != nil {
		return err
	}
	s.forgetAgentBrowserActivity(id)
	return nil
}

func (s *Service) ensureAgentBrowserStarted(id ID, startID int64, m Meta) {
	if err := s.containerBrowser.Ensure(context.Background(), m.ContainerName); err != nil {
		log.Printf("projects: start agent browser for %s: %v", id, err)
		s.finishAgentBrowserStart(id, startID, s.agentBrowserInfoFor(m, AgentBrowserStatusError, err.Error()))
		return
	}
	s.finishAgentBrowserStart(id, startID, s.agentBrowserInfoFor(m, AgentBrowserStatusReady, ""))
}

func (s *Service) agentBrowserInfoFor(m Meta, status AgentBrowserStatus, errMsg string) AgentBrowserInfo {
	info := AgentBrowserInfo{
		Status: status,
		Slug:   m.Slug,
		Error:  errMsg,
	}
	if s.containerBrowser != nil {
		info.Port = s.containerBrowser.Port()
	}
	return info
}

func (s *Service) agentBrowserState(id ID) (AgentBrowserInfo, bool) {
	s.agentBrowserMu.Lock()
	defer s.agentBrowserMu.Unlock()
	info, ok := s.agentBrowserInfo[id]
	return info, ok
}

func (s *Service) setAgentBrowserState(id ID, info AgentBrowserInfo) {
	s.agentBrowserMu.Lock()
	defer s.agentBrowserMu.Unlock()
	s.ensureAgentBrowserStateLocked()
	s.agentBrowserInfo[id] = info
}

func (s *Service) beginAgentBrowserStart(id ID, m Meta) (AgentBrowserInfo, int64) {
	s.agentBrowserMu.Lock()
	defer s.agentBrowserMu.Unlock()
	s.ensureAgentBrowserStateLocked()
	s.agentBrowserStart[id]++
	startID := s.agentBrowserStart[id]
	info := s.agentBrowserInfoFor(m, AgentBrowserStatusStarting, "")
	s.agentBrowserInfo[id] = info
	return info, startID
}

func (s *Service) finishAgentBrowserStart(id ID, startID int64, info AgentBrowserInfo) {
	s.agentBrowserMu.Lock()
	defer s.agentBrowserMu.Unlock()
	s.ensureAgentBrowserStateLocked()
	if s.agentBrowserStart[id] != startID {
		return
	}
	s.agentBrowserInfo[id] = info
}

func (s *Service) clearAgentBrowserState(id ID) {
	s.agentBrowserMu.Lock()
	defer s.agentBrowserMu.Unlock()
	s.ensureAgentBrowserStateLocked()
	s.agentBrowserStart[id]++
	delete(s.agentBrowserInfo, id)
}

func (s *Service) ensureAgentBrowserStateLocked() {
	if s.agentBrowserInfo == nil {
		s.agentBrowserInfo = make(map[ID]AgentBrowserInfo)
	}
	if s.agentBrowserStart == nil {
		s.agentBrowserStart = make(map[ID]int64)
	}
}

// StopAgentBrowserView tears down only the human noVNC layer.
func (s *Service) StopAgentBrowserView(ctx context.Context, id ID) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if s.containerBrowser == nil || m.ContainerName == "" {
		return nil
	}
	return s.containerBrowser.StopView(ctx, m.ContainerName)
}

// StartAgentBrowserReaper stops browser stacks that have had no agent or pane
// activity for ttl. It is safe to call multiple times; only the first call
// starts a ticker.
func (s *Service) StartAgentBrowserReaper(ctx context.Context, ttl time.Duration) {
	if ttl <= 0 || s.containerBrowser == nil {
		return
	}
	s.browserActivityMu.Lock()
	if s.reaperOn {
		s.browserActivityMu.Unlock()
		return
	}
	s.reaperOn = true
	s.browserActivityMu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.reapIdleAgentBrowsers(ttl)
			}
		}
	}()
}

func (s *Service) reapIdleAgentBrowsers(ttl time.Duration) {
	metas, err := s.repo.List(context.Background())
	if err != nil {
		log.Printf("projects: browser reaper list: %v", err)
		return
	}
	now := time.Now()
	for _, m := range metas {
		if m.ContainerName == "" {
			continue
		}
		s.browserActivityMu.Lock()
		last, ok := s.browserSeen[m.ID]
		if !ok || last.IsZero() {
			if s.browserSeen == nil {
				s.browserSeen = map[ID]time.Time{}
			}
			s.browserSeen[m.ID] = now
			s.browserActivityMu.Unlock()
			continue
		}
		idle := now.Sub(last)
		s.browserActivityMu.Unlock()
		if idle < ttl {
			continue
		}
		statusCtx, statusCancel := context.WithTimeout(context.Background(), 10*time.Second)
		info, statusErr := s.containerBrowser.Status(statusCtx, m.ContainerName)
		statusCancel()
		if statusErr != nil {
			continue
		}
		if info.Core != "ready" {
			s.clearAgentBrowserState(m.ID)
			s.forgetAgentBrowserActivity(m.ID)
			continue
		}
		if info.ViewerCount > 0 {
			s.TouchAgentBrowserActivity(context.Background(), m.ID)
			continue
		}
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := s.containerBrowser.Stop(stopCtx, m.ContainerName)
		stopCancel()
		if err != nil {
			log.Printf("projects: browser reap %s/%s after %s: %v", m.ID, m.ContainerName, idle.Round(time.Second), err)
		} else {
			s.clearAgentBrowserState(m.ID)
			s.forgetAgentBrowserActivity(m.ID)
			log.Printf("projects: browser reap %s/%s after %s", m.ID, m.ContainerName, idle.Round(time.Second))
		}
	}
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
			effective := s.effectiveLimits(ctx, m.ResourceLimits)
			if !limitsEmpty(effective) {
				if err := s.containerLifecycle.SetResourceLimits(ctx, m.ContainerName, effective); err != nil {
					log.Printf("projects: reconcile resource limits %s/%s: %v", m.ID, m.ContainerName, err)
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
	if s.access == nil {
		return false, nil
	}
	em := strings.ToLower(strings.TrimSpace(email))
	if em == "" {
		return false, nil
	}
	return s.access.Has(ctx, id, em)
}

// ListAccess returns the sorted, normalized membership list for a project.
func (s *Service) ListAccess(ctx context.Context, id ID) ([]string, error) {
	if !ValidID(id) {
		return nil, ErrInvalidID
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	if s.access == nil {
		return nil, nil
	}
	return s.access.List(ctx, id)
}

// AddAccess adds email to the project's membership list. Caller is
// responsible for verifying the email belongs to a registered user.
func (s *Service) AddAccess(ctx context.Context, id ID, email string) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		return err
	}
	if s.access == nil {
		return errors.New("access store unavailable")
	}
	em := strings.ToLower(strings.TrimSpace(email))
	if em == "" {
		return errors.New("empty email")
	}
	return s.access.Add(ctx, id, em)
}

// RemoveAccess deletes email from the project's membership list.
func (s *Service) RemoveAccess(ctx context.Context, id ID, email string) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		return err
	}
	if s.access == nil {
		return nil
	}
	em := strings.ToLower(strings.TrimSpace(email))
	if em == "" {
		return nil
	}
	return s.access.Remove(ctx, id, em)
}

// CountAccess returns how many members the project has.
func (s *Service) CountAccess(ctx context.Context, id ID) (int, error) {
	if !ValidID(id) {
		return 0, ErrInvalidID
	}
	if s.access == nil {
		return 0, nil
	}
	list, err := s.access.List(ctx, id)
	if err != nil {
		return 0, err
	}
	return len(list), nil
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

// syncContainerEnv pushes every stored secret for the project into the
// container's LXD environment.* config. Best-effort; logs failures.
func (s *Service) syncContainerEnv(ctx context.Context, id ID, containerName string) error {
	if s.containerEnvironment == nil || containerName == "" {
		return nil
	}
	if s.secrets == nil {
		return nil
	}
	secs, err := s.secrets.List(ctx, id)
	if err != nil {
		return err
	}
	if len(secs) == 0 {
		return nil
	}
	set := make(map[string]string, len(secs))
	for _, sec := range secs {
		set[sec.Key] = sec.Value
	}
	return s.containerEnvironment.ApplyDiff(ctx, containerName, set, nil)
}

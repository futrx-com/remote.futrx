// Package lifecycle owns project-container state transitions and launch-time
// orchestration without depending on LXD or host-filesystem details.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const (
	containerWorkspacePath = "/workspace"
	agentHomeDirName       = "agent-home"
)

var ErrContainerBusy = errors.New("container has an active agent process")

type Runtime interface {
	Available() bool
	Init(ctx context.Context, image, containerName string) error
	Disk(ctx context.Context, container, deviceName string) (hostSource, containerPath string, exists bool, err error)
	AttachDisk(ctx context.Context, container, deviceName, hostSource, containerPath string, readonly bool) error
	RemoveDevice(ctx context.Context, container, deviceName string) error
	PullDirectory(ctx context.Context, container, source, hostTarget string) (bool, error)
	Mounted(ctx context.Context, container, containerPath string) (bool, error)
	Busy(ctx context.Context, container string) (bool, error)
	EnsureBootAutostart(ctx context.Context, containerName string) error
	Start(ctx context.Context, containerName string) error
	Stop(ctx context.Context, containerName string) error
	Restart(ctx context.Context, containerName string) error
	Delete(ctx context.Context, containerName string) error
	State(ctx context.Context, containerName string) (serviceproject.ContainerState, error)
}

type WorkspacePreparer interface {
	Prepare(path string) error
}

type ResourceEnsurer interface {
	Ensure(ctx context.Context, containerName string) error
	SetLimits(ctx context.Context, containerName, cpu, memory, disk string) error
}

type LaunchProvisioner interface {
	Provision(ctx context.Context, containerName, displayName string)
}

type ProfileSource interface {
	Snapshot() []provisioning.Profile
}

type Service struct {
	runtime     Runtime
	image       string
	workspace   WorkspacePreparer
	resources   ResourceEnsurer
	provisioner LaunchProvisioner
	profiles    ProfileSource
}

func NewService(
	runtime Runtime,
	image string,
	workspace WorkspacePreparer,
	resources ResourceEnsurer,
	provisioner LaunchProvisioner,
	profiles ProfileSource,
) *Service {
	return &Service{
		runtime:     runtime,
		image:       image,
		workspace:   workspace,
		resources:   resources,
		provisioner: provisioner,
		profiles:    profiles,
	}
}

func (s *Service) Available() bool {
	return s.runtime.Available()
}

func (s *Service) Busy(ctx context.Context, containerName string) (bool, error) {
	if !s.runtime.Available() {
		return false, errors.New("lxc not available")
	}
	return s.runtime.Busy(ctx, containerName)
}

type durableMount struct {
	device          string
	hostPath        string
	containerPath   string
	migrateExisting bool
}

// Ensure converges a project to one complete, running container. Project
// creation, ordinary starts, recovery, and workspace upgrades all use this
// single path.
func (s *Service) Ensure(ctx context.Context, project serviceproject.Meta) error {
	if !s.runtime.Available() {
		return errors.New("lxc CLI not found on PATH - install LXD on the host first")
	}

	mounts, agentRoot, err := persistentMounts(project, s.profileSnapshot())
	if err != nil {
		return err
	}

	state, err := s.runtime.State(ctx, project.ContainerName)
	if err != nil {
		return err
	}
	if state == serviceproject.ContainerStateUnknown {
		return errors.New("container state is unknown")
	}
	created := state == serviceproject.ContainerStateMissing
	if created {
		if err := s.workspace.Prepare(project.Cwd); err != nil {
			return err
		}
		if err := s.preparePrivateDirectory(agentRoot); err != nil {
			return fmt.Errorf("prepare agent home: %w", err)
		}
		for _, mount := range mounts {
			if !mount.migrateExisting {
				continue
			}
			if err := s.preparePrivateDirectory(mount.hostPath); err != nil {
				return fmt.Errorf("prepare %s: %w", mount.device, err)
			}
		}
		if err := s.runtime.Init(ctx, s.image, project.ContainerName); err != nil {
			// A canceled or failed `lxc init` may still have created the instance.
			return s.rollbackNewContainer(ctx, project.ContainerName, err)
		}
		state = serviceproject.ContainerStateStopped
	}

	changes, err := s.mountChanges(ctx, project.ContainerName, mounts)
	if err != nil {
		return s.launchError(ctx, project.ContainerName, created, err)
	}
	if !created && len(changes) > 0 {
		busy, err := s.runtime.Busy(ctx, project.ContainerName)
		if err != nil {
			return err
		}
		if busy {
			return ErrContainerBusy
		}
		for _, mount := range changes {
			if mount.migrateExisting {
				if err := s.preparePrivateDirectory(agentRoot); err != nil {
					return fmt.Errorf("prepare agent home: %w", err)
				}
				break
			}
		}
		for _, mount := range changes {
			if !mount.migrateExisting {
				if err := s.workspace.Prepare(mount.hostPath); err != nil {
					return fmt.Errorf("prepare %s: %w", mount.device, err)
				}
			}
		}
		if state == serviceproject.ContainerStateFrozen {
			if err := s.runtime.Restart(ctx, project.ContainerName); err != nil {
				return err
			}
			state = serviceproject.ContainerStateRunning
		}
		if state == serviceproject.ContainerStateStopped {
			if err := s.runtime.Start(ctx, project.ContainerName); err != nil {
				return err
			}
			state = serviceproject.ContainerStateRunning
		}
		if err := s.migratePersistentState(ctx, project.ContainerName, changes); err != nil {
			return err
		}
	}

	if len(changes) > 0 && state == serviceproject.ContainerStateRunning {
		if err := s.runtime.Stop(ctx, project.ContainerName); err != nil {
			return s.launchError(ctx, project.ContainerName, created, err)
		}
		state = serviceproject.ContainerStateStopped
	}
	for _, mount := range changes {
		_, _, exists, err := s.runtime.Disk(ctx, project.ContainerName, mount.device)
		if err != nil {
			return s.launchError(ctx, project.ContainerName, created, err)
		}
		if exists {
			if err := s.runtime.RemoveDevice(ctx, project.ContainerName, mount.device); err != nil {
				return s.launchError(ctx, project.ContainerName, created, err)
			}
		}
		if err := s.runtime.AttachDisk(
			ctx,
			project.ContainerName,
			mount.device,
			mount.hostPath,
			mount.containerPath,
			false,
		); err != nil {
			return s.launchError(ctx, project.ContainerName, created, fmt.Errorf("attach %s: %w", mount.device, err))
		}
	}

	_ = s.resources.Ensure(ctx, project.ContainerName)
	if project.ResourceLimits != nil {
		if err := s.resources.SetLimits(
			ctx,
			project.ContainerName,
			project.ResourceLimits.CPU,
			project.ResourceLimits.Memory,
			project.ResourceLimits.Disk,
		); err != nil {
			return s.launchError(ctx, project.ContainerName, created, fmt.Errorf("apply resource limits: %w", err))
		}
	}

	_ = s.EnsureBootAutostart(ctx, project.ContainerName)
	if state == serviceproject.ContainerStateFrozen {
		if err := s.runtime.Restart(ctx, project.ContainerName); err != nil {
			return s.launchError(ctx, project.ContainerName, created, err)
		}
		state = serviceproject.ContainerStateRunning
	}
	if state != serviceproject.ContainerStateRunning {
		if err := s.runtime.Start(ctx, project.ContainerName); err != nil {
			return s.launchError(ctx, project.ContainerName, created, err)
		}
	}
	if err := s.verifyMounts(ctx, project.ContainerName, mounts); err != nil {
		if created {
			return s.rollbackNewContainer(ctx, project.ContainerName, err)
		}
		busy, busyErr := s.runtime.Busy(ctx, project.ContainerName)
		if busyErr != nil {
			return errors.Join(err, busyErr)
		}
		if busy {
			return errors.Join(err, ErrContainerBusy)
		}
		// Device configuration can be correct while a stale running instance
		// has not activated it. One host-side restart converges that case.
		if restartErr := s.runtime.Restart(ctx, project.ContainerName); restartErr != nil {
			return errors.Join(err, restartErr)
		}
		if retryErr := s.verifyMounts(ctx, project.ContainerName, mounts); retryErr != nil {
			return retryErr
		}
	}
	if created || len(changes) > 0 {
		s.provisioner.Provision(ctx, project.ContainerName, project.Name)
	}
	return nil
}

func (s *Service) profileSnapshot() []provisioning.Profile {
	if s.profiles == nil {
		return nil
	}
	return s.profiles.Snapshot()
}

func persistentMounts(project serviceproject.Meta, profiles []provisioning.Profile) ([]durableMount, string, error) {
	workspace := filepath.Clean(project.Cwd)
	if !filepath.IsAbs(workspace) || filepath.Base(workspace) != "workspace" {
		return nil, "", fmt.Errorf("invalid project workspace %q", project.Cwd)
	}
	if project.ContainerName == "" {
		return nil, "", errors.New("project has no container name")
	}
	agentRoot := filepath.Join(filepath.Dir(workspace), agentHomeDirName)
	mounts := []durableMount{{
		device: "workspace", hostPath: workspace, containerPath: containerWorkspacePath,
	}}
	for _, profile := range profiles {
		for _, state := range profile.PersistentState {
			if err := state.Validate(); err != nil {
				return nil, "", fmt.Errorf("invalid persistent state for agent %q: %w", profile.ID, err)
			}
			for _, mount := range mounts {
				if mount.device == state.Device || mount.hostPath == filepath.Join(agentRoot, state.HostDirectory) ||
					containerPathsOverlap(mount.containerPath, state.ContainerPath) {
					return nil, "", fmt.Errorf("persistent state for agent %q conflicts with mount %q", profile.ID, mount.device)
				}
			}
			mounts = append(mounts, durableMount{
				device:          state.Device,
				hostPath:        filepath.Join(agentRoot, state.HostDirectory),
				containerPath:   state.ContainerPath,
				migrateExisting: true,
			})
		}
	}
	return mounts, agentRoot, nil
}

func containerPathsOverlap(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func (s *Service) mountChanges(ctx context.Context, container string, mounts []durableMount) ([]durableMount, error) {
	changes := make([]durableMount, 0, len(mounts))
	for _, mount := range mounts {
		source, path, exists, err := s.runtime.Disk(ctx, container, mount.device)
		if err != nil {
			return nil, err
		}
		if !exists || filepath.Clean(source) != filepath.Clean(mount.hostPath) || filepath.Clean(path) != filepath.Clean(mount.containerPath) {
			changes = append(changes, mount)
		}
	}
	return changes, nil
}

func (s *Service) migratePersistentState(ctx context.Context, container string, changes []durableMount) error {
	for _, mount := range changes {
		if !mount.migrateExisting {
			continue
		}
		empty, err := directoryEmpty(mount.hostPath)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", mount.hostPath, err)
		}
		if !empty {
			if err := s.preparePrivateDirectory(mount.hostPath); err != nil {
				return fmt.Errorf("prepare migrated %s: %w", mount.device, err)
			}
			continue
		}
		staging, err := os.MkdirTemp(filepath.Dir(mount.hostPath), ".migration-")
		if err != nil {
			return fmt.Errorf("stage %s migration: %w", mount.device, err)
		}
		pulled, pullErr := s.runtime.PullDirectory(ctx, container, mount.containerPath, staging)
		if pullErr == nil && pulled {
			source := filepath.Join(staging, filepath.Base(mount.containerPath))
			if err := os.Remove(mount.hostPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				pullErr = fmt.Errorf("replace empty %s: %w", mount.hostPath, err)
			} else if err := os.Rename(source, mount.hostPath); err != nil {
				pullErr = fmt.Errorf("install migrated %s: %w", mount.device, err)
			}
		}
		_ = os.RemoveAll(staging)
		if pullErr != nil {
			return pullErr
		}
		if err := s.preparePrivateDirectory(mount.hostPath); err != nil {
			return fmt.Errorf("prepare migrated %s: %w", mount.device, err)
		}
	}
	return nil
}

func (s *Service) preparePrivateDirectory(path string) error {
	if err := s.workspace.Prepare(path); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func directoryEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func (s *Service) verifyMounts(ctx context.Context, container string, mounts []durableMount) error {
	for _, mount := range mounts {
		mounted, err := s.runtime.Mounted(ctx, container, mount.containerPath)
		if err != nil {
			return err
		}
		if !mounted {
			return fmt.Errorf("required mount %s is not active at %s", mount.device, mount.containerPath)
		}
	}
	return nil
}

func (s *Service) launchError(ctx context.Context, containerName string, created bool, cause error) error {
	if created {
		// Restore MISSING so the next recovery retries the complete sequence.
		return s.rollbackNewContainer(ctx, containerName, cause)
	}
	return cause
}

// rollbackNewContainer restores the pre-create invariant: when provisioning a
// new instance fails, the next Start must observe MISSING and retry the complete
// convergence sequence. Cleanup deliberately outlives request cancellation; the
// runtime's own delete deadline keeps it bounded.
func (s *Service) rollbackNewContainer(ctx context.Context, containerName string, cause error) error {
	if rollbackErr := s.runtime.Delete(context.WithoutCancel(ctx), containerName); rollbackErr != nil {
		return errors.Join(cause, fmt.Errorf("rollback launched container: %w", rollbackErr))
	}
	return cause
}

func (s *Service) EnsureBootAutostart(ctx context.Context, containerName string) error {
	if !s.runtime.Available() {
		return errors.New("lxc not available")
	}
	return s.runtime.EnsureBootAutostart(ctx, containerName)
}

// EnsureResources converges the fleet-default resource envelope (managed
// LXD profile) onto a container. Exposed so the project service's startup
// reconcile can converge the whole fleet — every deploy restarts the
// backend, so `update.sh` alone brings any box's containers to the pins.
func (s *Service) EnsureResources(ctx context.Context, containerName string) error {
	if !s.runtime.Available() {
		return errors.New("lxc not available")
	}
	return s.resources.Ensure(ctx, containerName)
}

func (s *Service) SetResourceLimits(
	ctx context.Context,
	containerName string,
	limits serviceproject.ContainerLimits,
) error {
	if !s.runtime.Available() {
		return errors.New("lxc not available")
	}
	if err := s.resources.Ensure(ctx, containerName); err != nil {
		return err
	}
	return s.resources.SetLimits(ctx, containerName, limits.CPU, limits.Memory, limits.Disk)
}

func (s *Service) Start(ctx context.Context, containerName string) error {
	// Converge the resource envelope on every explicit start, not only in
	// Launch: the project service short-circuits to Start for containers
	// that already exist. Best-effort — a profile hiccup must not block
	// the start; the startup reconcile sweep retries and logs.
	_ = s.resources.Ensure(ctx, containerName)
	return s.runtime.Start(ctx, containerName)
}

func (s *Service) Stop(ctx context.Context, containerName string) error {
	return s.runtime.Stop(ctx, containerName)
}

// Restart force-restarts a container from the host — the regain-control
// path for a workspace wedged at its resource limits (a host-kernel kill
// needs no cooperation from processes inside). Converges the resource
// envelope after the fresh boot.
func (s *Service) Restart(ctx context.Context, containerName string) error {
	if !s.runtime.Available() {
		return errors.New("lxc not available")
	}
	if err := s.runtime.Restart(ctx, containerName); err != nil {
		return err
	}
	_ = s.resources.Ensure(ctx, containerName)
	return nil
}

func (s *Service) Delete(ctx context.Context, containerName string) error {
	return s.runtime.Delete(ctx, containerName)
}

func (s *Service) State(ctx context.Context, containerName string) (serviceproject.ContainerState, error) {
	return s.runtime.State(ctx, containerName)
}

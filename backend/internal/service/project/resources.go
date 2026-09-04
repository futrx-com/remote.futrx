package project

import (
	"context"
	"strconv"
	"strings"
)

// StartOptions carries the per-request knobs of a run-state transition. Force
// is the admin escape hatch past the aggregate admission guard.
type StartOptions struct {
	Force bool
}

// policySnapshot returns the fleet policy, or a zero snapshot when no policy
// source is wired (tests, and any deployment without the resource service).
func (s *Service) policySnapshot(ctx context.Context) ContainerPolicySnapshot {
	if s.containerPolicy == nil {
		return ContainerPolicySnapshot{}
	}
	return s.containerPolicy.Policy(ctx)
}

// effectiveLimits resolves what LXD should enforce on one project: the
// project's own override wins field by field, and every field it leaves empty
// falls back to the fleet default.
//
// Resolving here rather than in the LXD adapter is deliberate. Only this layer
// knows which values came from a project override and which from the fleet, so
// only this layer can reapply a changed fleet default to a container that
// never had an override of its own.
func (s *Service) effectiveLimits(ctx context.Context, overrides *ContainerLimits) ContainerLimits {
	defaults := s.policySnapshot(ctx).Defaults
	effective := defaults
	if overrides == nil {
		return effective
	}
	if value := strings.TrimSpace(overrides.CPU); value != "" {
		effective.CPU = value
	}
	if value := strings.TrimSpace(overrides.Memory); value != "" {
		effective.Memory = value
	}
	if value := strings.TrimSpace(overrides.Disk); value != "" {
		effective.Disk = value
	}
	return effective
}

// withEffectiveLimits returns a copy of meta whose ResourceLimits carry the
// resolved envelope. The copy is what the lifecycle applies; the stored
// project keeps only its explicit overrides.
func (s *Service) withEffectiveLimits(ctx context.Context, meta Meta) Meta {
	effective := s.effectiveLimits(ctx, meta.ResourceLimits)
	meta.ResourceLimits = &effective
	return meta
}

// authorizeStart runs the aggregate admission guard for one project. A denial
// is returned verbatim so the transport layer can map the resource service's
// sentinel errors onto a 409.
func (s *Service) authorizeStart(ctx context.Context, meta Meta, force bool) error {
	if s.containerAdmission == nil || meta.ContainerName == "" {
		return nil
	}
	return s.containerAdmission.AuthorizeStart(
		ctx,
		meta.ContainerName,
		s.effectiveLimits(ctx, meta.ResourceLimits).Memory,
		force,
	)
}

// Resources reports one project's resource envelope: its explicit overrides,
// what LXD enforces after inheritance, the fleet policy bounding the form, and
// a live usage snapshot when the container is running. Readable by any project
// member; only an admin may change it.
func (s *Service) Resources(ctx context.Context, id ID, editable bool) (ProjectResources, error) {
	if !ValidID(id) {
		return ProjectResources{}, ErrInvalidID
	}
	meta, err := s.repo.Get(ctx, id)
	if err != nil {
		return ProjectResources{}, err
	}

	out := ProjectResources{
		Policy:    s.policySnapshot(ctx),
		Overrides: meta.ResourceLimits,
		Effective: s.effectiveLimits(ctx, meta.ResourceLimits),
		State:     ContainerStateUnknown,
		Editable:  editable,
	}
	if s.containerInspector == nil || meta.ContainerName == "" {
		return out, nil
	}
	info, err := s.containerInspector.Inspect(ctx, meta.ContainerName)
	if err != nil {
		// Diagnostics are best-effort: the policy half of the answer is still
		// worth returning when LXD is unreachable.
		return out, nil
	}
	out.State = info.State
	out.Usage = info.Resources
	if info.Limits != nil {
		// What LXD reports it is enforcing beats what we computed.
		out.Effective = *info.Limits
	}
	return out, nil
}

// SetResources validates a per-project override against the fleet ceiling and
// host capacity, applies it to an existing container, and persists it for the
// next launch. Memory and CPU are hot-settable, so they take effect
// immediately; a changed disk quota on a running container needs a restart,
// which the response reports rather than performing.
func (s *Service) SetResources(ctx context.Context, id ID, limits ContainerLimits) (ProjectResources, error) {
	if !ValidID(id) {
		return ProjectResources{}, ErrInvalidID
	}
	normalized, err := normalizeContainerLimits(limits)
	if err != nil {
		return ProjectResources{}, err
	}
	if s.containerPolicy != nil {
		if err := s.containerPolicy.Validate(ctx, normalized); err != nil {
			return ProjectResources{}, err
		}
	}
	meta, err := s.repo.Get(ctx, id)
	if err != nil {
		return ProjectResources{}, err
	}

	previousDisk := s.effectiveLimits(ctx, meta.ResourceLimits).Disk
	stored := normalized
	applied, state, err := s.applyLimits(ctx, meta, &stored)
	if err != nil {
		return ProjectResources{}, err
	}

	meta, err = s.repo.Update(ctx, id, func(project *Meta) {
		if limitsEmpty(stored) {
			project.ResourceLimits = nil
			return
		}
		saved := stored
		project.ResourceLimits = &saved
	})
	if err != nil {
		return ProjectResources{}, err
	}

	out, err := s.Resources(ctx, id, true)
	if err != nil {
		return ProjectResources{}, err
	}
	out.AppliedNow = applied
	nextDisk := s.effectiveLimits(ctx, meta.ResourceLimits).Disk
	out.NeedsRestart = applied &&
		state == ContainerStateRunning &&
		nextDisk != previousDisk &&
		out.Policy.DiskQuota.Supported
	return out, nil
}

// applyLimits pushes the effective envelope onto an existing container.
// A missing container is not an error: the desired values are persisted and
// the next launch applies them.
func (s *Service) applyLimits(ctx context.Context, meta Meta, overrides *ContainerLimits) (bool, ContainerState, error) {
	if s.containerLifecycle == nil || meta.ContainerName == "" {
		return false, ContainerStateUnknown, nil
	}
	state, err := s.containerLifecycle.State(ctx, meta.ContainerName)
	if err != nil {
		return false, ContainerStateUnknown, err
	}
	if state == ContainerStateMissing {
		return false, state, nil
	}
	effective := s.effectiveLimits(ctx, overrides)
	if err := s.containerLifecycle.SetResourceLimits(ctx, meta.ContainerName, effective); err != nil {
		return false, state, err
	}
	return true, state, nil
}

// FormatCores renders a policy core count for the ContainerLimits string form
// the rest of the project surface uses.
func FormatCores(cpu float64) string {
	if cpu <= 0 {
		return ""
	}
	cores := int(cpu)
	if float64(cores) < cpu {
		cores++
	}
	if cores < 1 {
		cores = 1
	}
	return strconv.Itoa(cores)
}

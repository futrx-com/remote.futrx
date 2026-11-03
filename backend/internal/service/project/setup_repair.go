package project

import (
	"context"
	"errors"
	"fmt"
)

// StartForAgent keeps a failed project script from blocking the agent needed
// to repair it. Infrastructure errors still stop preparation, and bypass is
// permitted only after checking that the container is actually running.
func (s *Service) StartForAgent(ctx context.Context, id ID) (Meta, error) {
	if !ValidID(id) {
		return Meta{}, ErrInvalidID
	}
	unlock := s.runState.lock(id)
	defer unlock()
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return Meta{}, err
	}
	if m.NeedsSetupRepair() && s.containerLifecycle != nil {
		state, err := s.containerLifecycle.State(ctx, m.ContainerName)
		if err != nil {
			return Meta{}, err
		}
		if state == ContainerStateRunning {
			if err := s.containerLifecycle.Ensure(ctx, m); err != nil {
				return s.setStartError(ctx, id, err)
			}
			return m, nil
		}
	}
	m, err = s.startLocked(ctx, id)
	if errors.Is(err, ErrSetupFailed) && m.NeedsSetupRepair() && ctx.Err() == nil && s.containerLifecycle != nil {
		state, stateErr := s.containerLifecycle.State(ctx, m.ContainerName)
		if stateErr == nil && state == ContainerStateRunning {
			return m, nil
		}
	}
	return m, err
}

// ClaimSetupRepair records admission before an agent can run. There is one
// automatic attempt per failed container setup; explicit Start still retries
// setup, and a later container replacement starts a new recovery opportunity.
func (s *Service) ClaimSetupRepair(ctx context.Context, id ID, chatID string) (bool, error) {
	if !ValidID(id) {
		return false, ErrInvalidID
	}
	unlock := s.runState.lock(id)
	defer unlock()
	claimed := false
	meta, err := s.repo.Get(ctx, id)
	if err != nil {
		return false, err
	}
	if s.containerLifecycle == nil {
		return false, nil
	}
	state, err := s.containerLifecycle.State(ctx, meta.ContainerName)
	if err != nil {
		return false, err
	}
	if state != ContainerStateRunning {
		return false, nil
	}
	busy, err := s.containerLifecycle.Busy(ctx, meta.ContainerName)
	if err != nil || busy {
		return false, err
	}
	_, err = s.repo.Update(ctx, id, func(m *Meta) {
		if m.NeedsSetupRepair() && (m.SetupRepair == nil || m.SetupRepair.State == "blocked") {
			m.SetupRepair = &SetupRepair{ChatID: chatID, State: "running"}
			claimed = true
		}
	})
	return claimed && err == nil, err
}

func (s *Service) SetSetupRepair(ctx context.Context, id ID, repair *SetupRepair) error {
	if !ValidID(id) {
		return ErrInvalidID
	}
	unlock := s.runState.lock(id)
	defer unlock()
	_, err := s.repo.Update(ctx, id, func(m *Meta) {
		if repair == nil {
			m.SetupRepair = nil
			return
		}
		copy := *repair
		m.SetupRepair = &copy
	})
	return err
}

// VerifySetup checks the repair in the current container and restores installed
// applications. It neither recreates nor restarts a container.
func (s *Service) VerifySetup(ctx context.Context, id ID) (Meta, error) {
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
		return Meta{}, errors.New("container lifecycle unavailable")
	}
	state, err := s.containerLifecycle.State(ctx, m.ContainerName)
	if err != nil {
		return Meta{}, err
	}
	if state != ContainerStateRunning {
		return Meta{}, fmt.Errorf("cannot verify setup in a %s container", state)
	}
	verifier, ok := s.containerSetup.(ContainerSetupVerifier)
	if !ok {
		return Meta{}, errors.New("setup verification unavailable")
	}
	environment, err := s.setupEnvironment(ctx, id)
	if err != nil {
		return Meta{}, err
	}
	if err := verifier.Verify(ctx, m.ContainerName, environment); err != nil {
		if ctx.Err() != nil {
			return m, err
		}
		return s.setStartError(ctx, id, err)
	}
	state, err = s.containerLifecycle.State(ctx, m.ContainerName)
	if err != nil {
		return Meta{}, err
	}
	if state != ContainerStateRunning {
		return s.setStartError(ctx, id, errors.New("container stopped during setup verification"))
	}
	if s.containerRestorer != nil {
		if err := s.containerRestorer(ctx, string(id)); err != nil {
			return s.setStartError(ctx, id, fmt.Errorf("restore applications: %w", err))
		}
	}
	return s.setSetupComplete(ctx, id)
}

func (s *Service) setSetupComplete(ctx context.Context, id ID) (Meta, error) {
	return s.repo.Update(ctx, id, func(m *Meta) {
		m.Status, m.ErrorMsg = StatusRunning, ""
		m.SetupFailed = false
		if m.SetupRepair != nil {
			m.SetupRepair = &SetupRepair{ChatID: m.SetupRepair.ChatID, State: "succeeded", Message: "Project setup was repaired and verified."}
		}
	})
}

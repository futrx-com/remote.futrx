package applications

import (
	"context"
	"errors"
	"fmt"
)

// RestoreProject reprovisions running applications after the project's root
// filesystem is replaced. Their instance records live outside the container;
// without this step the UI would claim they are running while their packages
// and systemd units no longer exist. Stopped copies stay stopped.
func (s *Service) RestoreProject(ctx context.Context, projectID string) error {
	instances, err := s.store.ListProject(ctx, projectID)
	if err != nil {
		return err
	}
	var failures []error
	for _, snapshot := range instances {
		if snapshot.Status != StatusRunning {
			continue
		}
		err := s.restoreProjectInstance(ctx, snapshot.ID)
		if err != nil {
			failures = append(failures, fmt.Errorf("restore application %s: %w", snapshot.ApplicationID, err))
		}
	}
	return errors.Join(failures...)
}

func (s *Service) restoreProjectInstance(ctx context.Context, instanceID string) error {
	unlock, acquired := s.instanceLocks.tryLock(instanceID)
	if !acquired {
		return nil
	}
	defer unlock()
	inst, application, err := s.load(ctx, instanceID)
	if err == nil && inst.Status == StatusRunning && application.NeedsContainer() {
		if s.installer == nil {
			err = ErrUnavailable
		} else if err = reconcileInstanceEnv(application, &inst); err == nil {
			err = s.stopBackend(ctx, application, inst)
		}
		if err == nil {
			err = s.installer.Install(ctx, InstallSpec{Application: application, Instance: inst})
		}
		if err == nil {
			inst.ApplicationVersion = application.Version
			inst.ContainerBuildVersion = application.containerBuildVersion()
			err = s.saveStatus(ctx, &inst, StatusRunning, "")
		}
		if err != nil {
			_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
		}
	}
	return err
}

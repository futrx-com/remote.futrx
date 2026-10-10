package applications

import (
	"context"
	"errors"
	"fmt"
)

// installIntoEveryProject is the global placement for an application that
// lives inside project containers: it creates an ordinary project instance in
// each existing project rather than one instance in a dedicated container,
// then records the global install as an in-projects instance. That record is
// what the global list shows and what a project created later inherits from.
//
// A project that already holds the application is skipped. If any project
// fails nothing is recorded, so repeating the request retries the failures.
func (s *Service) installIntoEveryProject(ctx context.Context, req InstallRequest, application Application) (View, error) {
	if s.projects == nil {
		return View{}, ErrUnavailable
	}
	if err := s.claimInstallSlot(ctx, ScopeGlobal, "", application.ID); err != nil {
		return View{}, err
	}
	// Reject bad inputs once here rather than once per project, and even when
	// there is no project yet to reject them.
	if _, err := resolveEnv(application, req.Env); err != nil {
		return View{}, err
	}
	projectIDs, err := s.projects.ListProjectIDs(ctx)
	if err != nil {
		return View{}, err
	}
	var failures []error
	for _, projectID := range projectIDs {
		projectReq := req
		projectReq.Scope, projectReq.ProjectID = ScopeProject, projectID
		_, err := s.installInstance(ctx, projectReq, application)
		if err != nil && !errors.Is(err, ErrAlreadyInstalled) {
			failures = append(failures, fmt.Errorf("project %s: %w", projectID, err))
		}
	}
	if len(failures) > 0 {
		return View{}, errors.Join(failures...)
	}

	// Env keeps the request's inputs, not resolved values, so each project
	// generates its own secrets when its copy is installed.
	global := Instance{
		ID:                 newInstanceID(),
		ApplicationID:      application.ID,
		ApplicationVersion: application.Version,
		Name:               displayName(req.Name, application.Name),
		Scope:              ScopeGlobal,
		Env:                req.Env,
		Status:             StatusInProjects,
		CreatedAt:          s.now(),
		UpdatedAt:          s.now(),
	}
	if err := s.store.Put(ctx, global); err != nil {
		return View{}, err
	}
	return s.view(global), nil
}

// InstallGlobalApplications gives one project its copy of every application
// installed globally inside project containers. The project service calls it
// for a newly created project, which no earlier global install could reach.
func (s *Service) InstallGlobalApplications(ctx context.Context, projectID string) error {
	globals, err := s.store.ListGlobal(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, global := range globals {
		if global.Status != StatusInProjects {
			continue
		}
		application, ok := s.registry.Get(global.ApplicationID)
		if !ok {
			continue
		}
		_, err := s.installInstance(ctx, InstallRequest{
			ApplicationID: application.ID,
			Scope:         ScopeProject,
			ProjectID:     projectID,
			Name:          global.Name,
			Env:           global.Env,
		}, application)
		if err != nil && !errors.Is(err, ErrAlreadyInstalled) {
			failures = append(failures, fmt.Errorf("application %s: %w", application.ID, err))
		}
	}
	return errors.Join(failures...)
}

// uninstallFromEveryProject removes an application's copy from each project,
// which is what uninstalling its global in-projects record means. A failure
// leaves that record in place so the uninstall can be repeated.
func (s *Service) uninstallFromEveryProject(ctx context.Context, applicationID string) error {
	instances, err := s.store.ListAll(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, inst := range instances {
		if inst.Scope != ScopeProject || inst.ApplicationID != applicationID {
			continue
		}
		if err := s.Uninstall(ctx, inst.ID); err != nil && !errors.Is(err, ErrNotFound) {
			failures = append(failures, fmt.Errorf("project %s: %w", inst.ProjectID, err))
		}
	}
	return errors.Join(failures...)
}

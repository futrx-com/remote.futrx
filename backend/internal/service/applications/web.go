package applications

import "context"

func (s *Service) ProjectWebTarget(ctx context.Context, projectID, applicationID string) (WebTarget, bool, error) {
	instances, err := s.store.ListProject(ctx, projectID)
	if err != nil {
		return WebTarget{}, false, err
	}
	for _, instance := range instances {
		if instance.ApplicationID == applicationID && instance.ProjectID == projectID {
			if target, ok := s.webTarget(instance); ok {
				return target, true, nil
			}
		}
	}
	return WebTarget{}, false, nil
}

func (s *Service) WebTarget(ctx context.Context, instanceID string) (WebTarget, bool, error) {
	instance, ok, err := s.store.Get(ctx, instanceID)
	if err != nil || !ok {
		return WebTarget{}, false, err
	}
	target, ok := s.webTarget(instance)
	return target, ok, nil
}

func (s *Service) webTarget(instance Instance) (WebTarget, bool) {
	application, ok := s.registry.Get(instance.ApplicationID)
	if !ok || application.Web == nil || instance.Scope != ScopeProject ||
		instance.ProjectID == "" || instance.Status != StatusRunning {
		return WebTarget{}, false
	}
	return WebTarget{Subdomain: application.Web.Subdomain, InstanceID: instance.ID, ProjectID: instance.ProjectID, Port: application.Web.Port}, true
}

// ProjectWebTargetBySubdomain resolves a label only inside the selected project.
// Ambiguous labels fail closed instead of choosing an arbitrary installation.
func (s *Service) ProjectWebTargetBySubdomain(ctx context.Context, projectID, label string) (WebTarget, bool, error) {
	instances, err := s.store.ListProject(ctx, projectID)
	if err != nil {
		return WebTarget{}, false, err
	}
	var result WebTarget
	found := false
	for _, instance := range instances {
		target, ok := s.webTarget(instance)
		if ok && target.ProjectID == projectID && target.Subdomain == label {
			if found {
				return WebTarget{}, false, nil
			}
			result, found = target, true
		}
	}
	return result, found, nil
}

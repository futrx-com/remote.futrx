package applications

import "context"

// WebTarget contains only the routing identity of a running project web app.
// Caller/project authorization belongs to the transport; secrets never leave here.
type WebTarget struct {
	InstanceID string
	ProjectID  string
	Port       int
}

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
	return WebTarget{InstanceID: instance.ID, ProjectID: instance.ProjectID, Port: application.Web.Port}, true
}

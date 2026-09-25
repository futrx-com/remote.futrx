package applications

import "context"

func (s *Service) publishApplicationAdded(ctx context.Context, applicationID string) {
	if s.lifecycle != nil {
		s.lifecycle.PublishApplicationAdded(ctx, applicationID)
	}
}

func (s *Service) publishApplicationUpdated(ctx context.Context, applicationID string) {
	if s.lifecycle != nil {
		s.lifecycle.PublishApplicationUpdated(ctx, applicationID)
	}
}

func (s *Service) publishApplicationDeleted(ctx context.Context, applicationID string) {
	if s.lifecycle != nil {
		s.lifecycle.PublishApplicationDeleted(ctx, applicationID)
	}
}

func (s *Service) publishApplicationInstalled(ctx context.Context, instance Instance) {
	if s.lifecycle != nil {
		s.lifecycle.PublishApplicationInstalled(
			ctx,
			instance.ApplicationID,
			instance.ID,
			string(instance.Scope),
			instance.ProjectID,
		)
	}
}

func (s *Service) publishApplicationUninstalled(ctx context.Context, instance Instance) {
	if s.lifecycle != nil {
		s.lifecycle.PublishApplicationUninstalled(
			ctx,
			instance.ApplicationID,
			instance.ID,
			string(instance.Scope),
			instance.ProjectID,
		)
	}
}

func (s *Service) publishApplicationTransition(
	ctx context.Context,
	instance Instance,
	previous, target InstanceStatus,
) {
	if s.lifecycle == nil || previous == target {
		return
	}
	arguments := []string{
		instance.ApplicationID,
		instance.ID,
		string(instance.Scope),
		instance.ProjectID,
	}
	switch target {
	case StatusRunning:
		s.lifecycle.PublishApplicationStarted(ctx, arguments[0], arguments[1], arguments[2], arguments[3])
	case StatusStopped:
		s.lifecycle.PublishApplicationStopped(ctx, arguments[0], arguments[1], arguments[2], arguments[3])
	}
}

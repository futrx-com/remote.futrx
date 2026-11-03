package prompt

import (
	"context"
	"errors"
	"sync"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

var ErrSetupRepairRunning = errors.New("project setup is being repaired; follow the Automatic setup repair chat and retry when it finishes")

type projectAdmission struct {
	count     int
	exclusive bool
}
type projectAdmissions struct {
	mu     sync.Mutex
	active map[servicechat.ProjectID]projectAdmission
}

func (s *Service) ProjectBusy(id serviceproject.ID) bool {
	s.projectRuns.mu.Lock()
	defer s.projectRuns.mu.Unlock()
	return s.projectRuns.active[servicechat.ProjectID(id)].count > 0
}

func (s *Service) admitProject(ctx context.Context, input StartInput) (func(), error) {
	meta, err := s.store.Get(ctx, input.ChatID)
	if err != nil {
		return nil, err
	}
	input.ExclusiveProject = input.ExclusiveProject || meta.SetupRepair
	if meta.ProjectID == "" {
		if input.ExclusiveProject || input.VerifyProjectSetup {
			return nil, errors.New("setup recovery requires a project")
		}
		return func() {}, nil
	}
	s.projectRuns.mu.Lock()
	defer s.projectRuns.mu.Unlock()
	active := s.projectRuns.active[meta.ProjectID]
	if active.exclusive {
		return nil, ErrSetupRepairRunning
	}
	if input.ExclusiveProject && active.count > 0 {
		return nil, serviceproject.ErrProjectBusy
	}
	if s.projectRuns.active == nil {
		s.projectRuns.active = make(map[servicechat.ProjectID]projectAdmission)
	}
	active.count++
	active.exclusive = input.ExclusiveProject
	s.projectRuns.active[meta.ProjectID] = active
	return func() {
		s.projectRuns.mu.Lock()
		defer s.projectRuns.mu.Unlock()
		active := s.projectRuns.active[meta.ProjectID]
		active.count--
		if active.count == 0 {
			delete(s.projectRuns.active, meta.ProjectID)
		} else {
			s.projectRuns.active[meta.ProjectID] = active
		}
	}, nil
}

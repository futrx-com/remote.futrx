package chat

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// TrustedWorkspaceRoot resolves filesystem authority for an already-loaded
// chat from server-owned state. Meta.Cwd is deliberately ignored: clients may
// set it to choose a working directory, so it cannot authorize host reads.
//
// A project record owns its workspace path. Loose chats have no project record
// and use the deployment's configured host workspace instead.
func (s *Service) TrustedWorkspaceRoot(ctx context.Context, meta Meta) (string, error) {
	root := s.hostWorkspaceRoot
	if meta.ProjectID != "" {
		if s.projects == nil {
			return "", fmt.Errorf("%w: project resolver is unavailable", ErrWorkspaceUnavailable)
		}
		projectRoot, err := s.projects.WorkspaceForProject(ctx, meta.ProjectID)
		if err != nil {
			return "", fmt.Errorf("%w: resolve project %s: %v", ErrWorkspaceUnavailable, meta.ProjectID, err)
		}
		root = projectRoot
	}

	root = filepath.Clean(strings.TrimSpace(root))
	if !filepath.IsAbs(root) {
		return "", ErrWorkspaceUnavailable
	}
	return root, nil
}

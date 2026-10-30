package project

import (
	"context"
	"errors"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"testing"
)

func TestCapabilitiesDenyBeforeRepositoryOrContainerIO(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  rbac.Key
		call func(*Service) error
	}{
		{"secrets read", PermissionSecretsManage, func(s *Service) error { _, e := s.ListSecrets(context.Background(), "abcd"); return e }},
		{"secrets set", PermissionSecretsManage, func(s *Service) error { _, e := s.SetSecret(context.Background(), "abcd", "KEY", "private"); return e }},
		{"secrets delete", PermissionSecretsManage, func(s *Service) error { return s.DeleteSecret(context.Background(), "abcd", "KEY") }},
		{"rename", PermissionControlsManage, func(s *Service) error { _, e := s.Update(context.Background(), "abcd", UpdateInput{}); return e }},
		{"delete", PermissionControlsManage, func(s *Service) error { return s.Delete(context.Background(), "abcd") }},
		{"limits", PermissionControlsManage, func(s *Service) error {
			_, e := s.SetContainerLimits(context.Background(), "abcd", ContainerLimits{})
			return e
		}},
		{"upgrade", PermissionControlsManage, func(s *Service) error { _, e := s.Upgrade(context.Background(), "abcd", false); return e }},
		{"browser start", "workspace.browser.use", func(s *Service) error { _, e := s.StartAgentBrowser(context.Background(), "abcd"); return e }},
		{"browser status", "workspace.browser.use", func(s *Service) error { _, e := s.AgentBrowserStatus(context.Background(), "abcd"); return e }},
		{"browser stop", "workspace.browser.use", func(s *Service) error { return s.StopAgentBrowser(context.Background(), "abcd") }},
		{"browser view stop", "workspace.browser.use", func(s *Service) error { return s.StopAgentBrowserView(context.Background(), "abcd") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := &recordingAuthorizer{err: rbac.ErrDenied}
			s := New(nil, ContainerDependencies{}, nil, nil, WithAuthorizer(policy))
			if err := tc.call(s); !errors.Is(err, rbac.ErrDenied) {
				t.Fatal(err)
			}
			checks := policy.recorded()
			if len(checks) != 1 || checks[0].Permission != tc.key || checks[0].Scope != rbac.ProjectScope("abcd") {
				t.Fatal(checks)
			}
		})
	}
}

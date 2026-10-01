// Package accountaccess applies the existing RBAC policy to provider account
// selection and outward metadata. It owns no assignments or credential store.
package accountaccess

import (
	"context"
	"errors"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

type Service struct {
	authorizer agentauth.AccountAuthorizer
	bindings   map[agent.ProviderID]agentauth.Binding
}

func New(authorizer agentauth.AccountAuthorizer, bindings []agentauth.Binding) *Service {
	s := &Service{authorizer: authorizer, bindings: map[agent.ProviderID]agentauth.Binding{}}
	for _, binding := range bindings {
		s.bindings[binding.ID()] = binding
	}
	return s
}
func (s *Service) CanManage(ctx context.Context) (bool, error) {
	if s == nil || s.authorizer == nil {
		return false, rbac.ErrDenied
	}
	err := s.authorizer.Require(ctx, rbac.Check{Permission: agentauth.PermissionAccountsManage, Scope: rbac.PlatformScope()})
	if errors.Is(err, rbac.ErrDenied) {
		return false, nil
	}
	return err == nil, err
}
func (s *Service) RequireManage(ctx context.Context) error {
	allowed, err := s.CanManage(ctx)
	if err != nil {
		return err
	}
	if !allowed {
		return rbac.ErrDenied
	}
	return nil
}
func (s *Service) canUse(ctx context.Context, provider agent.ProviderID, id string) (bool, error) {
	if s == nil {
		return false, rbac.ErrDenied
	}
	err := agentauth.RequireAccountUse(ctx, s.authorizer, provider, id)
	if errors.Is(err, rbac.ErrDenied) {
		return false, nil
	}
	return err == nil, err
}

// Resolve checks the actual selected account. Callers may pin the returned ID
// for a run; the credential owner also re-checks under its account lock.
func (s *Service) Resolve(ctx context.Context, provider agent.ProviderID, id string) (string, error) {
	id = strings.TrimSpace(id)
	if !agentauth.RestrictedProvider(provider) {
		return id, nil
	}
	if s == nil {
		return "", rbac.ErrDenied
	}
	binding, ok := s.bindings[provider]
	if !ok {
		return "", rbac.ErrDenied
	}
	snapshot := binding.Snapshot()
	selected := id
	if selected == "" && snapshot.Accounts != nil {
		selected = snapshot.Accounts.ActiveAccountID
	}
	allowed, err := s.canUse(ctx, provider, selected)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", rbac.ErrDenied
	}
	if selected != "" {
		found := false
		if snapshot.Accounts != nil {
			for _, a := range snapshot.Accounts.Items {
				if a.ID == selected {
					found = true
					break
				}
			}
		}
		if !found {
			return "", agentauth.ErrAccountNotFound
		}
	}
	return selected, nil
}

// Visible hides all unauthorized account metadata and login-flow details.
func (s *Service) Visible(ctx context.Context, provider agent.ProviderID, snapshot agentauth.Snapshot) (agentauth.Snapshot, error) {
	if !agentauth.RestrictedProvider(provider) {
		return snapshot, nil
	}
	manage, err := s.CanManage(ctx)
	if err != nil {
		return agentauth.Snapshot{}, err
	}
	if manage {
		return snapshot, nil
	}
	visible := agentauth.Snapshot{}
	defaultID := ""
	if snapshot.Accounts != nil {
		defaultID = snapshot.Accounts.ActiveAccountID
	}
	defaultAllowed, err := s.canUse(ctx, provider, defaultID)
	if err != nil {
		return visible, err
	}
	visible.Authenticated = defaultAllowed && snapshot.Authenticated
	{
		accounts := agentauth.AccountsSnapshot{Items: []agentauth.Account{}, DefaultAllowed: &defaultAllowed}
		var items []agentauth.Account
		if snapshot.Accounts != nil {
			items = snapshot.Accounts.Items
		}
		for _, account := range items {
			allowed, err := s.canUse(ctx, provider, account.ID)
			if err != nil {
				return agentauth.Snapshot{}, err
			}
			if allowed {
				accounts.Items = append(accounts.Items, account)
				visible.Authenticated = true
			}
		}
		if defaultAllowed {
			accounts.ActiveAccountID = defaultID
		}
		visible.Accounts = &accounts
	}
	return visible, nil
}
func (s *Service) CanViewAccount(ctx context.Context, provider agent.ProviderID, id string) (bool, error) {
	if !agentauth.RestrictedProvider(provider) {
		return true, nil
	}
	return s.canUse(ctx, provider, id)
}

type Target struct {
	Provider  agent.ProviderID `json:"provider"`
	AccountID string           `json:"accountId"`
	Label     string           `json:"label"`
	Scope     rbac.Scope       `json:"scope"`
}

// Targets exposes the assignable account scopes to policy managers. The
// service reuses the policy service's management permissions for this read.
func (s *Service) Targets(ctx context.Context) ([]Target, error) {
	if s == nil || s.authorizer == nil {
		return nil, rbac.ErrDenied
	}
	allowed := false
	for _, key := range []rbac.Key{rbac.PermissionAssignmentsManage, rbac.PermissionRolesManage} {
		err := s.authorizer.Require(ctx, rbac.Check{Permission: key, Scope: rbac.PlatformScope()})
		if err == nil {
			allowed = true
			break
		}
		if !errors.Is(err, rbac.ErrDenied) {
			return nil, err
		}
	}
	if !allowed {
		return nil, rbac.ErrDenied
	}
	out := []Target{}
	for _, provider := range []agent.ProviderID{agent.ProviderClaude, agent.ProviderCodex} {
		b, ok := s.bindings[provider]
		if !ok {
			continue
		}
		out = append(out, Target{Provider: provider, Label: "Default host login", Scope: rbac.ProviderAccountScope(string(provider), "")})
		snapshot := b.Snapshot()
		if snapshot.Accounts != nil {
			for _, a := range snapshot.Accounts.Items {
				out = append(out, Target{Provider: provider, AccountID: a.ID, Label: a.Label, Scope: rbac.ProviderAccountScope(string(provider), a.ID)})
			}
		}
	}
	return out, nil
}

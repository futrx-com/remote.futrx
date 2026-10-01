package auth

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
)

const PermissionAccountUse rbac.Key = "agents.account.use"
const PermissionAccountsManage rbac.Key = "agents.accounts.manage"

func PermissionDefinitions() []rbac.Definition {
	return []rbac.Definition{
		{Key: PermissionAccountsManage, Description: "Manage provider accounts and inspect raw login status.", Scopes: []rbac.ScopeKind{rbac.ScopePlatform}, Baseline: rbac.BaselineAdmin},
		{Key: PermissionAccountUse, Description: "Use a saved provider account or its default host login.", Scopes: []rbac.ScopeKind{rbac.ScopeProviderAccount}, Baseline: rbac.BaselineNone, Delegable: true},
	}
}

type AccountAuthorizer interface {
	Require(context.Context, rbac.Check) error
}

func RestrictedProvider(provider agent.ProviderID) bool {
	return provider == agent.ProviderClaude || provider == agent.ProviderCodex
}
func RequireAccountUse(ctx context.Context, authorizer AccountAuthorizer, provider agent.ProviderID, accountID string) error {
	if !RestrictedProvider(provider) {
		return nil
	}
	if authorizer == nil {
		return rbac.ErrDenied
	}
	return authorizer.Require(ctx, rbac.Check{Permission: PermissionAccountUse, Scope: rbac.ProviderAccountScope(string(provider), accountID)})
}

func RequireAccountManagement(ctx context.Context, authorizer AccountAuthorizer, provider agent.ProviderID) error {
	if !RestrictedProvider(provider) {
		return nil
	}
	if authorizer == nil {
		return rbac.ErrDenied
	}
	return authorizer.Require(ctx, rbac.Check{Permission: PermissionAccountsManage, Scope: rbac.PlatformScope()})
}

// Package workspaceaccess declares UI workspace capabilities in the existing
// RBAC vocabulary. It stores no policy and does not mint trusted actors.
package workspaceaccess

import (
	"context"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
)

type Authorizer interface {
	Require(context.Context, rbac.Check) error
}

func PermissionDefinitions() []rbac.Definition {
	var out []rbac.Definition
	for _, capability := range []string{"terminal", "files", "git", "browser"} {
		out = append(out,
			rbac.Definition{Key: rbac.Key("workspace." + capability + ".use"), Description: "Use project " + capability + " controls.", Scopes: []rbac.ScopeKind{rbac.ScopeProject}, Baseline: rbac.BaselineProjectMember, Delegable: true},
			rbac.Definition{Key: rbac.Key("workspace.host" + capability + ".use"), Description: "Use host " + capability + " controls.", Scopes: []rbac.ScopeKind{rbac.ScopePlatform}, Baseline: rbac.BaselineAdmin, Delegable: true},
		)
	}
	return out
}

func Require(ctx context.Context, a Authorizer, capability, projectID string) error {
	if a == nil {
		return rbac.ErrDenied
	}
	scope := rbac.PlatformScope()
	if projectID != "" {
		scope = rbac.ProjectScope(projectID)
	} else {
		capability = "host" + capability
	}
	return a.Require(ctx, rbac.Check{Permission: rbac.Key("workspace." + capability + ".use"), Scope: scope})
}

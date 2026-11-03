package claude

import (
	"context"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
)

// Existing credential lifecycle tests explicitly allow access; policy tests
// inject real RBAC or a denying authorizer instead.
type allowAccountUse struct{}

func (allowAccountUse) Require(context.Context, rbac.Check) error { return nil }

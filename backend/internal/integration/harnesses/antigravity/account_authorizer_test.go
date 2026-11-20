package antigravity

import (
	"context"
	"errors"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

type runAccountAuthorizer struct {
	checks      []rbac.Check
	allowChecks int
}

func (a *runAccountAuthorizer) Require(_ context.Context, check rbac.Check) error {
	a.checks = append(a.checks, check)
	if len(a.checks) <= a.allowChecks {
		return nil
	}
	return rbac.ErrDenied
}

func TestRuntimeFactoryRequiresDefaultAccountUse(t *testing.T) {
	factory, err := NewFactory()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := agentmodule.NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}
	deny := &runAccountAuthorizer{}
	runtime, err := catalog.Build(agentmodule.BuildDependencies{AccountAuthorizer: deny})
	if err != nil {
		t.Fatal(err)
	}
	provider := runtime.Lookup(agent.ProviderAntigravity)
	if err := provider.Run(context.Background(), agent.RunRequest{ProjectID: "project"}, nil); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("denied run reached preparation: %v", err)
	}
	if len(deny.checks) != 1 || deny.checks[0].Permission != agentauth.PermissionAccountUse || deny.checks[0].Scope != rbac.ProviderAccountScope("antigravity", "") {
		t.Fatalf("account checks = %#v", deny.checks)
	}
	if err := (&Provider{}).Run(context.Background(), agent.RunRequest{}, nil); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("missing authorizer = %v", err)
	}
}

func TestRunRechecksDefaultAccountBeforeProcessStart(t *testing.T) {
	authorizer := &runAccountAuthorizer{allowChecks: 1}
	provider := &Provider{accountAuthorizer: authorizer}
	if err := provider.Run(context.Background(), agent.RunRequest{Cwd: t.TempDir()}, nil); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("revoked permission reached process start: %v", err)
	}
	if len(authorizer.checks) != 2 {
		t.Fatalf("account checks = %#v", authorizer.checks)
	}
}

func TestRunRejectsSavedAccountIDsWithoutUsingDefaultLogin(t *testing.T) {
	authorizer := &runAccountAuthorizer{allowChecks: 1}
	provider := &Provider{accountAuthorizer: authorizer}
	if err := provider.Run(context.Background(), agent.RunRequest{AccountID: "invented-account"}, nil); !errors.Is(err, agentauth.ErrAccountNotFound) {
		t.Fatalf("unsupported saved account = %v", err)
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Scope != rbac.ProviderAccountScope("antigravity", "") {
		t.Fatalf("account checks = %#v", authorizer.checks)
	}
}

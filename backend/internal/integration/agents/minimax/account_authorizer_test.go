package minimax

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

type runAccountAuthorizer struct{ checks []rbac.Check }

func (a *runAccountAuthorizer) Require(_ context.Context, check rbac.Check) error {
	a.checks = append(a.checks, check)
	return rbac.ErrDenied
}

func TestFactoryRequiresMiniMaxAccountUseBeforeModelDiscovery(t *testing.T) {
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
	provider := runtime.Lookup(agent.ProviderMiniMax)
	if err := provider.Run(context.Background(), agent.RunRequest{ProjectID: "project"}, nil); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("denied run reached model discovery: %v", err)
	}
	if len(deny.checks) != 1 || deny.checks[0].Permission != agentauth.PermissionAccountUse || deny.checks[0].Scope != rbac.ProviderAccountScope("minimax", "") {
		t.Fatalf("account checks = %#v", deny.checks)
	}
}

type revokingAPIKeySource struct {
	accountIDs []string
}

func (s *revokingAPIKeySource) APIKey() (string, bool) {
	panic("run bypassed authorized acquisition")
}

func (s *revokingAPIKeySource) APIKeyForRun(_ context.Context, id string) (agentauth.RunAPIKey, bool, error) {
	s.accountIDs = append(s.accountIDs, id)
	return agentauth.RunAPIKey{AccountID: "work", Key: "work-key"}, true, nil
}

func (s *revokingAPIKeySource) RequireRunAccess(_ context.Context, id string) error {
	s.accountIDs = append(s.accountIDs, id)
	return rbac.ErrDenied
}

func TestRunPinsResolvedMiniMaxAccountAndRechecksBeforeProcessStart(t *testing.T) {
	keys := &revokingAPIKeySource{}
	provider := newProvider(
		miniMaxTestPreparer{project: agent.PreparedProject{ContainerName: "project-container"}},
		keys,
		miniMaxTestModels{models: []string{"MiniMax-M2.7"}},
		&miniMaxTestRuntimeAssets{},
		"codex",
		nil,
	)
	if err := provider.Run(context.Background(), agent.RunRequest{ProjectID: "project"}, nil); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("revoked permission reached process start: %v", err)
	}
	if !reflect.DeepEqual(keys.accountIDs, []string{"", "work"}) {
		t.Fatalf("account was not pinned for the execution recheck: %#v", keys.accountIDs)
	}
}

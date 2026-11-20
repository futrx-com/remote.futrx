package minimax

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
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
	catalog, err := harnessmodule.NewCatalog(factory)
	if err != nil {
		t.Fatal(err)
	}
	deny := &runAccountAuthorizer{}
	runtime, err := catalog.Build(harnessmodule.BuildDependencies{AccountAuthorizer: deny})
	if err != nil {
		t.Fatal(err)
	}
	provider := runtime.Lookup(harness.ProviderMiniMax)
	if err := provider.Run(context.Background(), harness.RunRequest{ProjectID: "project"}, nil); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("denied run reached model discovery: %v", err)
	}
	if len(deny.checks) != 1 || deny.checks[0].Permission != harnessauth.PermissionAccountUse || deny.checks[0].Scope != rbac.ProviderAccountScope("minimax", "") {
		t.Fatalf("account checks = %#v", deny.checks)
	}
}

type revokingAPIKeySource struct {
	accountIDs []string
}

func (s *revokingAPIKeySource) APIKey() (string, bool) {
	panic("run bypassed authorized acquisition")
}

func (s *revokingAPIKeySource) APIKeyForRun(_ context.Context, id string) (harnessauth.RunAPIKey, bool, error) {
	s.accountIDs = append(s.accountIDs, id)
	return harnessauth.RunAPIKey{AccountID: "work", Key: "work-key"}, true, nil
}

func (s *revokingAPIKeySource) RequireRunAccess(_ context.Context, id string) error {
	s.accountIDs = append(s.accountIDs, id)
	return rbac.ErrDenied
}

func TestRunPinsResolvedMiniMaxAccountAndRechecksBeforeProcessStart(t *testing.T) {
	keys := &revokingAPIKeySource{}
	provider := newProvider(
		miniMaxTestPreparer{project: harness.PreparedProject{ContainerName: "project-container"}},
		keys,
		miniMaxTestModels{models: []string{"MiniMax-M2.7"}},
		&miniMaxTestRuntimeAssets{},
		"codex",
		nil,
	)
	if err := provider.Run(context.Background(), harness.RunRequest{ProjectID: "project"}, nil); !errors.Is(err, rbac.ErrDenied) {
		t.Fatalf("revoked permission reached process start: %v", err)
	}
	if !reflect.DeepEqual(keys.accountIDs, []string{"", "work"}) {
		t.Fatalf("account was not pinned for the execution recheck: %#v", keys.accountIDs)
	}
}

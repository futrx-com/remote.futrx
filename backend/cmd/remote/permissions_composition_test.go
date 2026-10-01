package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepermissions"
)

// membershipAccess answers Has from a fixed set of "<project>/<email>" pairs.
type membershipAccess struct {
	serviceproject.AccessRepository
	members map[string]bool
}

func (a membershipAccess) Has(_ context.Context, id serviceproject.ID, email string) (bool, error) {
	return a.members[string(id)+"/"+email], nil
}

func TestProjectMembershipAdaptsTheAccessRepository(t *testing.T) {
	membership := projectMembership{access: membershipAccess{members: map[string]bool{"beefcafe/member@example.com": true}}}

	for name, test := range map[string]struct {
		project, email string
		want           bool
	}{
		"a member":                       {"beefcafe", "member@example.com", true},
		"email case and padding ignored": {"beefcafe", "  Member@Example.com ", true},
		"another project":                {"deadbeef", "member@example.com", false},
		"a non-member":                   {"beefcafe", "other@example.com", false},
		"an empty email":                 {"beefcafe", " ", false},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := membership.HasAccess(context.Background(), test.project, test.email)
			if err != nil || got != test.want {
				t.Fatalf("HasAccess() = %v, %v; want %v", got, err, test.want)
			}
		})
	}

	got, err := (projectMembership{}).HasAccess(context.Background(), "beefcafe", "member@example.com")
	if err != nil || got {
		t.Fatalf("HasAccess() without a repository = %v, %v; want false", got, err)
	}
}

func TestPermissionCatalogRegistersEveryProjectPermission(t *testing.T) {
	registry, err := rbac.NewRegistry(permissionDefinitions()...)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []rbac.Key{
		serviceproject.PermissionLifecycleManage, serviceproject.PermissionAccessManage,
		rbac.PermissionAssignmentsManage, rbac.PermissionRolesManage,
		agentauth.PermissionAccountUse, agentauth.PermissionAccountsManage,
	} {
		if _, ok := registry.Lookup(key); !ok {
			t.Errorf("%s is not registered by the composition root", key)
		}
	}
}

// An empty policy starts; a corrupt policy file or an unregistered permission
// key refuses to.
type testIdentity struct{}

func (testIdentity) IsAdmin(context.Context, string) (bool, error)      { return false, nil }
func (testIdentity) IsRegistered(context.Context, string) (bool, error) { return true, nil }

func TestPermissionServiceStartupValidatesThePolicy(t *testing.T) {
	build := func(t *testing.T, content string) error {
		t.Helper()
		dir := t.TempDir()
		if content != "" {
			if err := os.WriteFile(filepath.Join(dir, "permissions.json"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		store, err := filepermissions.New(dir)
		if err != nil {
			return err
		}
		_, err = newPermissionService(context.Background(), store, testIdentity{}, nil)
		return err
	}

	if err := build(t, ""); err != nil {
		t.Fatalf("empty policy error = %v, want start", err)
	}
	if err := build(t, "{not json"); err == nil {
		t.Fatal("corrupt permissions.json started, want refusal")
	}
	unknown := `{"version":1,"assignments":[{"id":"a1","createdBy":"admin@example.com","createdAt":1,"userEmail":"a@example.com","permission":"nope.thing.do","effect":"allow","scope":{"kind":"platform"}}]}`
	if err := build(t, unknown); !errors.Is(err, rbac.ErrInvalidState) {
		t.Fatalf("unknown permission key error = %v, want ErrInvalidState", err)
	}
}

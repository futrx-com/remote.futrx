package chat

import (
	"context"
	"errors"
	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"testing"
)

type deniedAccountSelection struct {
	provider agent.ProviderID
	id       string
}

func (d *deniedAccountSelection) Resolve(_ context.Context, p agent.ProviderID, id string) (string, error) {
	d.provider = p
	d.id = id
	return "", rbac.ErrDenied
}
func TestChatAccountSelectionDeniesCreateForkAndChangesBeforeWrites(t *testing.T) {
	for _, operation := range []string{"create", "fork", "account", "provider"} {
		repo := &forkRepository{source: Meta{ID: "deadbeef", Provider: ProviderCodex, AccountID: "old"}}
		policy := &deniedAccountSelection{}
		service := New(repo, nil, nil, nil, WithAccountAccess(policy))
		ctx := rbac.ContextWithSystemActor(context.Background())
		var err error
		switch operation {
		case "create":
			_, err = service.Create(ctx, CreateInput{Provider: ProviderCodex, AccountID: "forged"})
		case "fork":
			_, err = service.Fork(ctx, "deadbeef")
		case "account":
			id := "forged"
			_, err = service.Update(ctx, "deadbeef", UpdateInput{AccountID: &id})
		case "provider":
			provider := ProviderClaude
			_, err = service.Update(ctx, "deadbeef", UpdateInput{Provider: &provider})
		}
		if !errors.Is(err, rbac.ErrDenied) {
			t.Fatalf("%s: %v", operation, err)
		}
		if repo.created.ID != "" || repo.source.AccountID != "old" {
			t.Fatalf("%s modified chat", operation)
		}
		if operation == "provider" && (policy.provider != agent.ProviderClaude || policy.id != "") {
			t.Fatalf("provider change must check its default: %+v", policy)
		}
	}
}

package prompt

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	agentaccountaccess "github.com/futrx-com/remote.futrx.com/internal/service/agent/accountaccess"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filepermissions"
)

type promptAccountCheck struct {
	mu         sync.Mutex
	actors     []string
	providers  []agent.ProviderID
	ids        []string
	resolvedID string
	denyAt     int
}

func (a *promptAccountCheck) Resolve(ctx context.Context, provider agent.ProviderID, id string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	actor, ok := rbac.ActorFromContext(ctx)
	if !ok {
		return "", rbac.ErrActorRequired
	}
	a.actors = append(a.actors, actor.Email)
	a.providers = append(a.providers, provider)
	a.ids = append(a.ids, id)
	if actor.Email == "" || (a.denyAt > 0 && len(a.ids) >= a.denyAt) {
		return "", rbac.ErrDenied
	}
	return a.resolvedID, nil
}

type accountRecordingProvider struct {
	usageProvider
	provider agent.ProviderID
	requests []agent.RunRequest
	actors   []string
}

func (p *accountRecordingProvider) ID() agent.ProviderID { return p.provider }

func (p *accountRecordingProvider) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	p.requests = append(p.requests, req)
	actor, _ := rbac.ActorFromContext(ctx)
	p.actors = append(p.actors, actor.Email)
	return p.usageProvider.Run(ctx, req, func(event agent.Event) {
		event.Provider = p.ID()
		emit(event)
	})
}

var promptAccountProviders = []struct {
	provider  agent.ProviderID
	accountID string
}{
	{agent.ProviderClaude, "team"},
	{agent.ProviderCodex, "team"},
	{agent.ProviderMiniMax, "team"},
	{agent.ProviderKimi, ""},
	{agent.ProviderAntigravity, ""},
}

func TestPromptAccountChecksKeepScheduledActorAndPinResolvedAccount(t *testing.T) {
	for _, tc := range promptAccountProviders {
		for _, scheduled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/scheduled=%t", tc.provider, scheduled), func(t *testing.T) {
				provider := &accountRecordingProvider{provider: tc.provider}
				access := &promptAccountCheck{resolvedID: tc.accountID}
				recorder := &runAuditRecorder{}
				service, _, meta := newUsagePromptService(t, provider, &recordingLedger{}, WithAccountAccess(access), WithApplicationToolIssuer(stubApplicationTools{}), WithAudit(recorder))
				input := StartInput{ChatID: meta.ID, Prompt: "hello", Actor: Actor{Email: "owner@example.com", IsAdmin: true}}
				if scheduled {
					input.ApplicationInstanceID = "scheduled-tasks-one"
					input.ApplicationID = "scheduled-tasks"
					input.ApplicationRequestID = "scheduled-run-one"
				}
				handle, err := service.Start(input, nil)
				if err != nil {
					t.Fatal(err)
				}
				if result := <-handle.Done; result.Err != nil {
					t.Fatal(result.Err)
				}
				if len(provider.requests) != 1 || provider.requests[0].AccountID != tc.accountID || provider.requests[0].Provider != tc.provider || provider.actors[0] != "owner@example.com" {
					t.Fatalf("provider request: %+v actors=%v", provider.requests, provider.actors)
				}
				for _, entry := range recorder.entries {
					if entry.Actor.Email != "owner@example.com" || entry.Meta["accountId"] != tc.accountID {
						t.Fatalf("audit correlation: %+v", entry)
					}
					if scheduled && (entry.Meta["applicationInstanceId"] != "scheduled-tasks-one" || entry.Meta["applicationRequestId"] != "scheduled-run-one") {
						t.Fatalf("scheduled audit correlation: %+v", entry)
					}
				}
				access.mu.Lock()
				defer access.mu.Unlock()
				if len(access.ids) < 3 || access.ids[len(access.ids)-1] != tc.accountID {
					t.Fatalf("missing run recheck: %v", access.ids)
				}
				for i, email := range access.actors {
					if email != "owner@example.com" || access.providers[i] != tc.provider {
						t.Fatalf("wrong actor/provider: %v %v", access.actors, access.providers)
					}
				}
			})
		}
	}
}
func TestPromptDenialBeforeAcceptanceAndRevocationBeforeRunLeaveNoPrompt(t *testing.T) {
	for _, tc := range promptAccountProviders {
		for _, denyAt := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/check=%d", tc.provider, denyAt), func(t *testing.T) {
				provider := &accountRecordingProvider{provider: tc.provider}
				access := &promptAccountCheck{denyAt: denyAt, resolvedID: tc.accountID}
				service, store, meta := newUsagePromptService(t, provider, &recordingLedger{}, WithAccountAccess(access))
				handle, err := service.Start(StartInput{ChatID: meta.ID, Prompt: "must not run", Actor: Actor{Email: "member@example.com"}}, nil)
				if denyAt == 1 {
					if !errors.Is(err, rbac.ErrDenied) {
						t.Fatalf("preflight: %v", err)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					if result := <-handle.Done; !errors.Is(result.Err, rbac.ErrDenied) {
						t.Fatalf("revocation: %v", result.Err)
					}
				}
				events, err := store.ReadEvents(context.Background(), meta.ID)
				if err != nil || len(events) != 0 || len(provider.requests) != 0 {
					t.Fatalf("denied run produced effects: events=%v calls=%d err=%v", events, len(provider.requests), err)
				}
				access.mu.Lock()
				defer access.mu.Unlock()
				for _, checked := range access.providers {
					if checked != tc.provider {
						t.Fatalf("checked provider = %q, want %q", checked, tc.provider)
					}
				}
			})
		}
	}
}

func TestPromptAccountRevocationAtExecutionStopsProvider(t *testing.T) {
	for _, tc := range promptAccountProviders {
		t.Run(string(tc.provider), func(t *testing.T) {
			provider := &accountRecordingProvider{provider: tc.provider}
			access := &promptAccountCheck{denyAt: 3, resolvedID: tc.accountID}
			service, _, meta := newUsagePromptService(t, provider, nil, WithAccountAccess(access))
			handle, err := service.Start(StartInput{ChatID: meta.ID, Prompt: "must not execute", Actor: Actor{Email: "member@example.com"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if result := awaitScheduleRun(t, handle); !errors.Is(result.Err, rbac.ErrDenied) {
				t.Fatalf("execution recheck: %v", result.Err)
			}
			if len(provider.requests) != 0 {
				t.Fatal("provider executed after account revocation")
			}
			access.mu.Lock()
			defer access.mu.Unlock()
			if len(access.ids) != 3 || access.providers[2] != tc.provider || access.ids[2] != tc.accountID {
				t.Fatalf("execution checked wrong selection: providers=%v ids=%v", access.providers, access.ids)
			}
		})
	}
}

type promptPolicyIdentities struct{}

func (promptPolicyIdentities) IsAdmin(_ context.Context, email string) (bool, error) {
	return email == "admin@example.com", nil
}
func (promptPolicyIdentities) IsRegistered(_ context.Context, email string) (bool, error) {
	return email == "admin@example.com" || email == "owner@example.com", nil
}

type promptSavedAccounts struct{ agentauth.AccountController }

func (promptSavedAccounts) AccountsSnapshot() agentauth.AccountsSnapshot {
	return agentauth.AccountsSnapshot{ActiveAccountID: "team", Items: []agentauth.Account{{ID: "team", Label: "Team", Active: true}}}
}

func TestPromptProviderAccountPolicyEnforcesGrantsAndRevocation(t *testing.T) {
	for _, tc := range promptAccountProviders[2:] {
		for _, scheduled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/scheduled=%t", tc.provider, scheduled), func(t *testing.T) {
				repo, err := filepermissions.New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				policy, err := rbac.NewService(context.Background(), rbac.MustRegistry(rbac.ManagementDefinitions(), agentauth.PermissionDefinitions()), repo, promptPolicyIdentities{}, nil)
				if err != nil {
					t.Fatal(err)
				}
				binding := agentauth.NewExternalBinding(tc.provider)
				if tc.accountID != "" {
					binding = binding.WithAccounts(promptSavedAccounts{})
				}
				access := agentaccountaccess.New(policy, []agentauth.Binding{binding})
				provider := &accountRecordingProvider{provider: tc.provider}
				service, store, meta := newUsagePromptService(t, provider, nil, WithAccountAccess(access), WithApplicationToolIssuer(stubApplicationTools{}))
				// Neither a trusted parent context nor an unverified IsAdmin hint may
				// elevate an interactive caller or a captured application owner.
				input := StartInput{ChatID: meta.ID, Prompt: "hello", Actor: Actor{Email: "owner@example.com", IsAdmin: true}, ParentContext: rbac.ContextWithSystemActor(context.Background())}
				if scheduled {
					input.ApplicationID = "scheduled-tasks"
					input.ApplicationInstanceID = "scheduled-tasks-one"
					input.ApplicationRequestID = "scheduled-run-one"
				}
				if _, err := service.Start(input, nil); !errors.Is(err, rbac.ErrDenied) {
					t.Fatalf("ungranted account: %v", err)
				}
				events, err := store.ReadEvents(context.Background(), meta.ID)
				if err != nil || len(events) != 0 || len(provider.requests) != 0 {
					t.Fatalf("denied admission produced effects: events=%v requests=%v err=%v", events, provider.requests, err)
				}
				admin := rbac.ContextWithActor(context.Background(), rbac.UserActor("admin@example.com"))
				assignment := rbac.AssignmentInput{UserEmail: "owner@example.com", Permission: agentauth.PermissionAccountUse, Scope: rbac.ProviderAccountScope(string(tc.provider), tc.accountID), Effect: rbac.Allow}
				if _, err := policy.SetAssignment(admin, assignment); err != nil {
					t.Fatal(err)
				}
				handle, err := service.Start(input, nil)
				if err != nil {
					t.Fatal(err)
				}
				if result := awaitScheduleRun(t, handle); result.Err != nil {
					t.Fatal(result.Err)
				}
				if len(provider.requests) != 1 || provider.requests[0].Provider != tc.provider || provider.requests[0].AccountID != tc.accountID || provider.actors[0] != "owner@example.com" {
					t.Fatalf("authorized run identity: requests=%+v actors=%v", provider.requests, provider.actors)
				}
				events, err = store.ReadEvents(context.Background(), meta.ID)
				if err != nil {
					t.Fatal(err)
				}
				assignment.Effect = rbac.Deny
				if _, err := policy.SetAssignment(admin, assignment); err != nil {
					t.Fatal(err)
				}
				if _, err := service.Start(input, nil); !errors.Is(err, rbac.ErrDenied) {
					t.Fatalf("revoked account: %v", err)
				}
				after, err := store.ReadEvents(context.Background(), meta.ID)
				if err != nil || len(after) != len(events) || len(provider.requests) != 1 {
					t.Fatalf("revoked admission produced effects: before=%d after=%d calls=%d err=%v", len(events), len(after), len(provider.requests), err)
				}
			})
		}
	}
}

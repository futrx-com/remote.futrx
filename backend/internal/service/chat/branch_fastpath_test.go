package chat

import (
	"context"
	"errors"
	"testing"

	permission "github.com/futrx-com/remote.futrx.com/internal/rbac"
)

// Any accidental use of the legacy copying path fails immediately, without
// allocating a fixture proportional to a long conversation's history.
type branchOnlyRepository struct {
	*forkRepository
	forkCalls   int
	rewindCalls int
	cutoff      int64
	forkMeta    Meta
	branchErr   error
	gate        *branchIdleGate
}

func (*branchOnlyRepository) ReadEvents(context.Context, ID) ([]Event, error) {
	panic("branch operation must not load full history")
}
func (*branchOnlyRepository) Create(context.Context, Meta) (Meta, error) {
	panic("branch operation must not create a copied chat")
}
func (*branchOnlyRepository) AppendEvent(context.Context, ID, Event) (Event, error) {
	panic("branch operation must not copy events")
}
func (*branchOnlyRepository) TruncateEventsBefore(context.Context, ID, int64) ([]Event, error) {
	panic("branch rewind must not materialize legacy history")
}
func (r *branchOnlyRepository) ForkChat(_ context.Context, id ID, meta Meta, cutoff int64) (Meta, error) {
	if id != r.source.ID {
		panic("wrong fork source")
	}
	r.forkCalls++
	r.cutoff = cutoff
	r.forkMeta = meta
	meta.ID = "feedface"
	return meta, r.branchErr
}
func (r *branchOnlyRepository) RewindBranch(_ context.Context, id ID, cutoff int64) error {
	if id != r.source.ID {
		panic("wrong rewind target")
	}
	if r.gate != nil && !r.gate.inside {
		panic("rewind happened outside idle gate")
	}
	r.rewindCalls++
	r.cutoff = cutoff
	return r.branchErr
}

type branchIdleGate struct {
	running, inside bool
	calls           int
}

func (g *branchIdleGate) IsRunning(ID) bool                { return g.running }
func (g *branchIdleGate) Cancel(context.Context, ID) error { return nil }
func (g *branchIdleGate) MutateIdle(_ ID, fn func() error) error {
	g.calls++
	if g.running {
		return ErrChatRunning
	}
	g.inside = true
	defer func() { g.inside = false }()
	return fn()
}

func TestForkUsesBranchPortWithoutReadingOrCopyingHistory(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			repo := &branchOnlyRepository{forkRepository: &forkRepository{source: Meta{
				ID: "deadbeef", Title: "Source", Provider: ProviderCodex, AccountID: "account", ProjectID: "project",
				Sessions: SessionIDs{ProviderCodex: "native-codex", ProviderClaude: "native-claude"},
			}}}
			if fail {
				repo.branchErr = errors.New("branch failed")
			}
			service := New(repo, nil, nil, nil, WithBranchForker(repo), WithSessionPolicy(forkSessionPolicy{string(ProviderCodex): true}))
			fork, err := service.Fork(permission.ContextWithSystemActor(context.Background()), "deadbeef")
			if !errors.Is(err, repo.branchErr) {
				t.Fatalf("error %v", err)
			}
			if repo.forkCalls != 1 || repo.cutoff != -1 {
				t.Fatalf("calls=%d cutoff=%d", repo.forkCalls, repo.cutoff)
			}
			if len(repo.forkMeta.Sessions) != 0 || repo.forkMeta.ForkPending || repo.forkMeta.CodexSessionID != "" {
				t.Fatalf("fork reused native context: %+v", repo.forkMeta)
			}
			if fork.Provider != ProviderCodex || fork.AccountID != "account" || fork.ProjectID != "project" {
				t.Fatalf("fork routing changed: %+v", fork)
			}
		})
	}
}

func TestRewindUsesBranchPortInsideIdleGateWithoutLoadingHistory(t *testing.T) {
	for _, tc := range []struct {
		name      string
		running   bool
		branchErr error
	}{
		{name: "success"}, {name: "running", running: true}, {name: "failed", branchErr: errors.New("rewind failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gate := &branchIdleGate{running: tc.running}
			repo := &branchOnlyRepository{forkRepository: &forkRepository{source: Meta{ID: "deadbeef"}}, gate: gate, branchErr: tc.branchErr}
			service := New(repo, nil, nil, gate, WithBranchRewinder(repo))
			events, err := service.Rewind(context.Background(), "deadbeef", 123)
			wantErr := tc.branchErr
			if tc.running {
				wantErr = ErrChatRunning
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("error=%v want=%v", err, wantErr)
			}
			if len(events) != 0 {
				t.Fatal("rewind returned full history")
			}
			if gate.calls != 1 {
				t.Fatalf("idle gate calls=%d", gate.calls)
			}
			wantCalls := 1
			if tc.running {
				wantCalls = 0
			}
			if repo.rewindCalls != wantCalls {
				t.Fatalf("branch calls=%d want=%d", repo.rewindCalls, wantCalls)
			}
		})
	}
}

func TestBranchForkStillAuthorizesBeforeStorage(t *testing.T) {
	repo := &branchOnlyRepository{forkRepository: &forkRepository{source: Meta{ID: "deadbeef", ProjectID: "project", Provider: ProviderCodex}}}
	service := New(repo, nil, nil, nil, WithBranchForker(repo), WithAuthorizer(&denyCreation{}))
	if _, err := service.Fork(context.Background(), "deadbeef"); !errors.Is(err, permission.ErrDenied) {
		t.Fatalf("error=%v", err)
	}
	if repo.forkCalls != 0 {
		t.Fatal("denied branch fork reached storage")
	}
}

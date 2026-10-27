package applications_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	project "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileapplicationturns"
	api "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type runtimeRegistry map[string]svc.Application

func (r runtimeRegistry) List() []svc.Application {
	var out []svc.Application
	for _, a := range r {
		out = append(out, a)
	}
	return out
}
func (r runtimeRegistry) Get(id string) (svc.Application, bool) { a, ok := r[id]; return a, ok }
func (r runtimeRegistry) UIAsset(string, string) ([]byte, bool) { return nil, false }

type runtimeStore struct {
	mu        sync.Mutex
	instances map[string]svc.Instance
}

func (s *runtimeStore) Get(_ context.Context, id string) (svc.Instance, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.instances[id]
	return i, ok, nil
}
func (s *runtimeStore) Put(_ context.Context, i svc.Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instances[i.ID] = i
	return nil
}
func (s *runtimeStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.instances, id)
	return nil
}
func (s *runtimeStore) ListAll(context.Context) ([]svc.Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []svc.Instance{}
	for _, i := range s.instances {
		out = append(out, i)
	}
	return out, nil
}
func (s *runtimeStore) ListGlobal(ctx context.Context) ([]svc.Instance, error) {
	all, _ := s.ListAll(ctx)
	out := []svc.Instance{}
	for _, i := range all {
		if i.Scope == svc.ScopeGlobal {
			out = append(out, i)
		}
	}
	return out, nil
}
func (s *runtimeStore) ListProject(ctx context.Context, id string) ([]svc.Instance, error) {
	all, _ := s.ListAll(ctx)
	out := []svc.Instance{}
	for _, i := range all {
		if i.Scope == svc.ScopeProject && i.ProjectID == id {
			out = append(out, i)
		}
	}
	return out, nil
}

type runtimeAccess struct {
	mu                        sync.Mutex
	registered, member, admin bool
}

func (a *runtimeAccess) IsRegistered(context.Context, string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.registered, nil
}
func (a *runtimeAccess) IsAdmin(context.Context, string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.admin, nil
}
func (a *runtimeAccess) HasAccess(context.Context, project.ID, string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.member, nil
}

type runtimeChats struct{ events []chat.Event }

func (c runtimeChats) Get(_ context.Context, id chat.ID) (chat.Meta, error) {
	p := chat.ProjectID("project")
	if id == "other" {
		p = "elsewhere"
	}
	return chat.Meta{ID: id, ProjectID: p}, nil
}
func (c runtimeChats) EventPage(_ context.Context, _ chat.ID, q chat.EventPageQuery) (chat.EventPage, error) {
	out := []chat.Event{}
	for _, e := range c.events {
		if q.BeforeSeq == 0 || e.Seq < q.BeforeSeq {
			out = append(out, e)
		}
	}
	return chat.EventPage{Events: out}, nil
}

type runtimePrompts struct {
	mu     sync.Mutex
	inputs []prompt.StartInput
	done   []chan prompt.RunResult
	err    error
}

func (p *runtimePrompts) Start(in prompt.StartInput, _ func(chat.Event)) (prompt.RunHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return prompt.RunHandle{}, p.err
	}
	done := make(chan prompt.RunResult, 1)
	p.inputs = append(p.inputs, in)
	p.done = append(p.done, done)
	return prompt.RunHandle{TurnID: fmt.Sprintf("turn-%d", len(p.inputs)), Done: done}, nil
}
func (p *runtimePrompts) count() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.inputs) }
func (p *runtimePrompts) finish(index int, result prompt.RunResult) {
	p.mu.Lock()
	done := p.done[index]
	p.mu.Unlock()
	done <- result
}

type runtimeHost struct {
	mu       sync.Mutex
	ensured  []string
	requests []api.Request
}

func (h *runtimeHost) Ensure(_ context.Context, i api.Instance) (api.Descriptor, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ensured = append(h.ensured, i.ID)
	return api.Descriptor{APIVersion: api.APIVersion}, nil
}
func (h *runtimeHost) Call(_ context.Context, _ api.Instance, r api.Request) (api.Response, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, r)
	return api.JSON(200, r.Agent), nil
}
func (h *runtimeHost) Notify(context.Context, api.Instance, api.Event) error { return nil }
func (h *runtimeHost) Stop(context.Context, string) error                    { return nil }
func (h *runtimeHost) Remove(context.Context, string) error                  { return nil }
func (h *runtimeHost) InvalidateApplication(string)                          {}

type runtimeFixture struct {
	registry runtimeRegistry
	store    *runtimeStore
	access   *runtimeAccess
	prompts  *runtimePrompts
	repo     svc.AgentTurnRepository
	host     *runtimeHost
}

func newRuntimeFixture(t *testing.T) *runtimeFixture {
	t.Helper()
	f := &runtimeFixture{registry: runtimeRegistry{}, store: &runtimeStore{instances: map[string]svc.Instance{}}, access: &runtimeAccess{registered: true, member: true}, prompts: &runtimePrompts{}, repo: fileapplicationturns.New(t.TempDir()), host: &runtimeHost{}}
	for _, id := range []string{"reminders", "build-monitor"} {
		f.registry[id] = svc.Application{ID: id, Scopes: []svc.Scope{svc.ScopeProject}, Backend: &svc.ApplicationBackend{AgentTurns: true, AgentTools: true, Background: true}, Skills: []string{id}}
		f.store.instances[id] = svc.Instance{ID: id, ApplicationID: id, Scope: svc.ScopeProject, ProjectID: "project", Status: svc.StatusRunning}
	}
	return f
}
func (f *runtimeFixture) service(t *testing.T) *svc.Service {
	t.Helper()
	s := svc.New(f.registry, f.store, nil, nil, nil, svc.WithBackendHost(f.host), svc.WithAgentRuntime(context.Background(), svc.AgentDependencies{Chats: runtimeChats{events: []chat.Event{{Seq: 1, TurnID: "turn-1", Text: "first"}, {Seq: 2, TurnID: "another", Text: "hidden"}, {Seq: 3, TurnID: "turn-1", Text: "last"}}}, Projects: f.access, Identities: f.access, Prompts: f.prompts, Turns: f.repo}), svc.WithAgentToolURL("https://remote.test"))
	t.Cleanup(s.Close)
	return s
}
func agentInput(id string) api.AgentTurnRequest {
	return api.AgentTurnRequest{RequestID: id, ChatID: "chat", OwnerEmail: "OWNER@example.com", Prompt: "Perform app work", Context: json.RawMessage(`{"job":"42"}`)}
}
func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

func TestAgentTurnsAreReusableScopedAndDurable(t *testing.T) {
	f := newRuntimeFixture(t)
	s := f.service(t)
	a := s.AgentTurnsForInstance("reminders")
	b := s.AgentTurnsForInstance("build-monitor")
	in := agentInput("job-1")
	first, err := a.Start(in)
	if err != nil || first.TurnID == "" {
		t.Fatalf("start: %+v %v", first, err)
	}
	replay, err := a.Start(in)
	if err != nil || replay.TurnID != first.TurnID || f.prompts.count() != 1 {
		t.Fatal("retry duplicated execution")
	}
	changed := in
	changed.Prompt = "different"
	if _, err := a.Start(changed); !errors.Is(err, api.ErrAgentRequestChanged) {
		t.Fatalf("changed input: %v", err)
	}
	if _, err := b.Read(api.AgentTurnQuery{RequestID: in.RequestID}); !errors.Is(err, api.ErrAgentTurnNotFound) {
		t.Fatalf("cross-instance receipt: %v", err)
	}
	if _, err := b.Start(in); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Start(agentInput("job-2")); !errors.Is(err, api.ErrAgentBusy) {
		t.Fatalf("admission limit: %v", err)
	}
	if err := a.Forget(in.RequestID); !errors.Is(err, api.ErrAgentBusy) {
		t.Fatalf("forgot active turn: %v", err)
	}
	f.prompts.finish(0, prompt.RunResult{Output: "job done"})
	eventually(t, func() bool {
		turn, err := a.Read(api.AgentTurnQuery{RequestID: in.RequestID, Limit: 1})
		return err == nil && turn.Status == "succeeded"
	})
	page, err := a.Read(api.AgentTurnQuery{RequestID: in.RequestID, Limit: 1})
	if err != nil || len(page.Events) != 1 || !page.HasMore || page.NextBefore != 3 {
		t.Fatalf("page: %+v %v", page, err)
	}
	next, err := a.Read(api.AgentTurnQuery{RequestID: in.RequestID, BeforeSeq: page.NextBefore, Limit: 1})
	if err != nil || len(next.Events) != 1 || next.HasMore || next.NextBefore != 1 {
		t.Fatalf("next page: %+v %v", next, err)
	}
	s.Close()
	restarted := f.service(t)
	replay, err = restarted.AgentTurnsForInstance("reminders").Start(in)
	if err != nil || replay.Status != "succeeded" || replay.Output != "job done" || f.prompts.count() != 2 {
		t.Fatalf("receipt lost on restart: %+v %v", replay, err)
	}
	pending, err := restarted.AgentTurnsForInstance("build-monitor").Read(api.AgentTurnQuery{RequestID: in.RequestID})
	if err != nil || pending.Status != "interrupted" {
		t.Fatalf("interrupted turn: %+v %v", pending, err)
	}
	if _, err := restarted.AgentTurnsForInstance("build-monitor").Start(in); err != nil || f.prompts.count() != 3 {
		t.Fatalf("interrupted recovery: %v", err)
	}
}
func TestAgentExecutionRechecksInstallationOwnerAndProject(t *testing.T) {
	f := newRuntimeFixture(t)
	s := f.service(t)
	a := s.AgentTurnsForInstance("reminders")
	in := agentInput("job")
	cross := in
	cross.ChatID = "other"
	if _, err := a.Start(cross); !errors.Is(err, api.ErrAgentAccess) {
		t.Fatalf("cross-project start: %v", err)
	}
	f.access.mu.Lock()
	f.access.member = false
	f.access.mu.Unlock()
	if _, err := a.Start(in); !errors.Is(err, api.ErrAgentAccess) {
		t.Fatalf("revoked member: %v", err)
	}
	f.access.mu.Lock()
	f.access.admin = true
	f.access.mu.Unlock()
	if _, err := a.Start(in); err != nil {
		t.Fatal(err)
	}
	f.access.mu.Lock()
	f.access.registered = false
	f.access.mu.Unlock()
	if _, err := a.Read(api.AgentTurnQuery{RequestID: "job"}); !errors.Is(err, api.ErrAgentAccess) {
		t.Fatalf("removed owner read: %v", err)
	}
	f.access.mu.Lock()
	f.access.registered = true
	f.access.mu.Unlock()
	i, _, _ := f.store.Get(context.Background(), "reminders")
	i.Status = svc.StatusStopped
	_ = f.store.Put(context.Background(), i)
	if _, err := a.Start(in); !errors.Is(err, api.ErrAgentAccess) {
		t.Fatalf("stopped backend: %v", err)
	}
}
func TestApplicationToolsFenceCallsAndStampOpaqueContext(t *testing.T) {
	f := newRuntimeFixture(t)
	s := f.service(t)
	request := prompt.ApplicationToolRequest{Actor: prompt.Actor{Email: "owner@example.com"}, ChatID: "chat", ProjectID: "project", ApplicationInstanceID: "reminders", ApplicationRequestID: "job", Context: json.RawMessage(`{"job":42}`)}
	access, err := s.IssueApplicationTools(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	token := access.Env["REMOTE_APPLICATION_GRANT"]
	forged := api.Request{Method: "POST", Path: "own-command", Agent: &api.AgentContext{ChatID: "forged"}, Caller: api.Caller{Email: "attacker", IsAdmin: true}}
	if _, err := s.CallAgentApplication(context.Background(), token, "build-monitor", forged); !errors.Is(err, api.ErrAgentAccess) {
		t.Fatalf("background crossed application: %v", err)
	}
	if _, err := s.CallAgentApplication(context.Background(), token, "reminders", forged); err != nil {
		t.Fatal(err)
	}
	f.host.mu.Lock()
	received := f.host.requests[0]
	f.host.mu.Unlock()
	if received.Caller.Email != "owner@example.com" || received.Caller.IsAdmin || received.Agent.ChatID != "chat" || !received.Agent.Background || received.Agent.RequestID != "job" || string(received.Agent.Context) != `{"job":42}` {
		t.Fatalf("host context: %+v", received)
	}
	if _, err := s.CallBackend(context.Background(), "reminders", forged, api.Caller{Email: "owner@example.com"}); err != nil {
		t.Fatal(err)
	}
	f.host.mu.Lock()
	browser := f.host.requests[1]
	f.host.mu.Unlock()
	if browser.Agent != nil {
		t.Fatal("browser forged background context")
	}
	f.access.mu.Lock()
	f.access.member = false
	f.access.mu.Unlock()
	if _, err := s.CallAgentApplication(context.Background(), token, "reminders", forged); !errors.Is(err, api.ErrAgentAccess) {
		t.Fatalf("stale membership: %v", err)
	}
	access.Revoke()
	if _, err := s.CallAgentApplication(context.Background(), token, "reminders", forged); !errors.Is(err, svc.ErrInvalidAgentGrant) {
		t.Fatalf("revoked grant: %v", err)
	}
}
func TestBackgroundRecoveryUsesManifestAndLifecycle(t *testing.T) {
	f := newRuntimeFixture(t)
	app := f.registry["build-monitor"]
	app.Backend = &svc.ApplicationBackend{}
	f.registry["build-monitor"] = app
	f.store.instances["stopped"] = svc.Instance{ID: "stopped", ApplicationID: "reminders", Scope: svc.ScopeProject, ProjectID: "project", Status: svc.StatusStopped}
	s := f.service(t)
	s.RestoreBackground(context.Background())
	s.RestoreBackground(context.Background())
	f.host.mu.Lock()
	defer f.host.mu.Unlock()
	if len(f.host.ensured) != 2 || f.host.ensured[0] != "reminders" || f.host.ensured[1] != "reminders" {
		t.Fatalf("background policy: %+v", f.host.ensured)
	}
}

func TestNoEligibleApplicationsDoNotBlockOrdinaryPromptTools(t *testing.T) {
	f := newRuntimeFixture(t)
	f.store.instances = map[string]svc.Instance{}
	f.access.registered = false
	s := f.service(t)
	access, err := s.IssueApplicationTools(context.Background(), prompt.ApplicationToolRequest{Actor: prompt.Actor{Email: "owner@example.com"}, ChatID: "chat", ProjectID: "project"})
	if err != nil || len(access.Env) != 0 {
		t.Fatalf("absent applications blocked ordinary turn: %+v %v", access, err)
	}
}

type failingReceipts struct {
	svc.AgentTurnRepository
	blocked  atomic.Bool
	attempts atomic.Int32
}

func (s *failingReceipts) Put(ctx context.Context, id string, record svc.AgentTurnRecord) error {
	if record.Turn.Status == "succeeded" || record.Turn.Status == "failed" {
		s.attempts.Add(1)
		if s.blocked.Load() {
			return errors.New("disk unavailable")
		}
	}
	return s.AgentTurnRepository.Put(ctx, id, record)
}
func TestResultWriteFailureKeepsAcceptedTurnAndAdmission(t *testing.T) {
	f := newRuntimeFixture(t)
	receipts := &failingReceipts{AgentTurnRepository: f.repo}
	receipts.blocked.Store(true)
	f.repo = receipts
	s := f.service(t)
	a := s.AgentTurnsForInstance("reminders")
	in := agentInput("job")
	if _, err := a.Start(in); err != nil {
		t.Fatal(err)
	}
	f.prompts.finish(0, prompt.RunResult{Output: "done"})
	eventually(t, func() bool { return receipts.attempts.Load() > 0 })
	if _, err := a.Start(in); err != nil || f.prompts.count() != 1 {
		t.Fatal("failed receipt write duplicated accepted turn")
	}
	if _, err := s.AgentTurnsForInstance("build-monitor").Start(agentInput("second")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Start(agentInput("third")); !errors.Is(err, api.ErrAgentBusy) {
		t.Fatalf("released admission before durability: %v", err)
	}
	receipts.blocked.Store(false)
	eventually(t, func() bool {
		turn, err := a.Read(api.AgentTurnQuery{RequestID: "job"})
		return err == nil && turn.Status == "succeeded"
	})
	if _, err := a.Start(agentInput("third")); err != nil {
		t.Fatal(err)
	}
}
func TestUninstallRemovesReceiptsAndLateCompletionCannotRestoreThem(t *testing.T) {
	f := newRuntimeFixture(t)
	s := f.service(t)
	if _, err := s.AgentTurnsForInstance("reminders").Start(agentInput("job")); err != nil {
		t.Fatal(err)
	}
	if err := s.Uninstall(context.Background(), "reminders"); err != nil {
		t.Fatal(err)
	}
	f.prompts.finish(0, prompt.RunResult{Output: "late result"})
	s.Close()
	if _, found, err := f.repo.Get(context.Background(), "reminders", "job"); err != nil || found {
		t.Fatalf("uninstalled receipt recreated: %t %v", found, err)
	}
}

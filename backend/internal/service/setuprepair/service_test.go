package setuprepair_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	"github.com/futrx-com/remote.futrx.com/internal/service/setuprepair"
	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
)

type lifecycle struct {
	serviceproject.ContainerLifecycle
}

func (lifecycle) Ensure(context.Context, serviceproject.Meta) error { return nil }
func (lifecycle) State(context.Context, string) (serviceproject.ContainerState, error) {
	return serviceproject.ContainerStateRunning, nil
}
func (lifecycle) Busy(context.Context, string) (bool, error) { return false, nil }

// Exercise real shell failure/repair, using an isolated workspace. The usersetup
// adapter's tests separately exercise its production wrapper and markers.
type shellSetup struct {
	workspace string
	calls     int
}

func (s *shellSetup) Ensure(ctx context.Context, _ string, _ map[string]string) error {
	s.calls++
	cmd := exec.CommandContext(ctx, "bash", "-e", filepath.Join(s.workspace, "setup.sh"))
	cmd.Dir = s.workspace
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", serviceproject.ErrSetupFailed, out)
	}
	return nil
}
func (s *shellSetup) Verify(ctx context.Context, name string, env map[string]string) error {
	return s.Ensure(ctx, name, env)
}

type identities struct{ registered, admin bool }

func (i *identities) IsRegistered(context.Context, string) (bool, error) { return i.registered, nil }
func (i *identities) IsAdmin(context.Context, string) (bool, error)      { return i.admin, nil }

type gate struct{ blocked bool }

func (g *gate) Blocked() bool { return g.blocked }

type repairAgent struct {
	projects          *serviceproject.Service
	workspace         string
	requests          []agent.RunRequest
	fail, leaveBroken bool
	entered           chan struct{}
	resume            <-chan struct{}
}

func (*repairAgent) ID() agent.ProviderID                     { return agent.ProviderCodex }
func (*repairAgent) Parser(agent.RunRequest) agent.LineParser { return nil }
func (*repairAgent) Capabilities(context.Context, agent.CapabilityRequest) (agent.Capabilities, error) {
	return agent.Capabilities{}, nil
}
func (p *repairAgent) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	p.requests = append(p.requests, req)
	if _, err := p.projects.StartForAgent(ctx, serviceproject.ID(req.ProjectID)); err != nil {
		return err
	}
	if p.entered != nil {
		close(p.entered)
	}
	if p.resume != nil {
		select {
		case <-p.resume:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if p.fail {
		return errors.New("agent authentication unavailable")
	}
	if !p.leaveBroken {
		if err := os.WriteFile(filepath.Join(p.workspace, "setup.sh"), []byte("#!/bin/bash\nprintf 'restored' > restored-tool\n"), 0700); err != nil {
			return err
		}
	}
	emit(agent.Event{Type: agent.EventRunCompleted, T: time.Now().UnixMilli(), Usage: json.RawMessage(`{"input_tokens":25,"output_tokens":10}`)})
	return nil
}

type usageRecorder struct{ events []serviceusage.RunEvent }

func (u *usageRecorder) RecordRun(_ context.Context, event serviceusage.RunEvent) {
	u.events = append(u.events, event)
}

type fixture struct {
	worker     *setuprepair.Service
	projects   *serviceproject.Service
	chats      *filechat.Store
	prompts    *prompt.Service
	project    serviceproject.Meta
	source     servicechat.Meta
	provider   *repairAgent
	setup      *shellSetup
	identities *identities
	gate       *gate
	usage      *usageRecorder
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	repo, err := fileproject.NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project, err := repo.Create(ctx, serviceproject.Meta{Name: "repair", Slug: "repair", Status: serviceproject.StatusRunning})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project.Cwd, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project.Cwd, "setup.sh"), []byte("#!/bin/bash\nbash missing-installer.sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	setup := &shellSetup{workspace: project.Cwd}
	projects := serviceproject.New(repo, serviceproject.ContainerDependencies{Lifecycle: lifecycle{}, Setup: setup}, nil, nil)
	project, err = projects.Start(ctx, project.ID)
	if !errors.Is(err, serviceproject.ErrSetupFailed) {
		t.Fatal(err)
	}
	chats, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, err := chats.Create(ctx, servicechat.Meta{Title: "existing project chat", ProjectID: servicechat.ProjectID(project.ID), Provider: agent.ProviderCodex, AccountID: "saved-account", Model: "chosen-model", Mode: "plan", ApprovalPolicy: "on-request", SandboxPolicy: "workspace-write", Cwd: project.Cwd})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chats.AppendEvent(ctx, source.ID, servicechat.Event{Type: "user", UserEmail: "member@example.test", Text: "work on my project"}); err != nil {
		t.Fatal(err)
	}
	provider := &repairAgent{projects: projects, workspace: project.Cwd}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	gate := &gate{}
	usage := &usageRecorder{}
	prompts := prompt.New(chats, nil, projects, runhub.New(chats), registry, prompt.WithStartGate(gate), prompt.WithUsageRecorder(usage))
	identities := &identities{registered: true, admin: true}
	f := &fixture{projects: projects, chats: chats, prompts: prompts, project: project, source: source, provider: provider, setup: setup, identities: identities, gate: gate, usage: usage}
	f.worker = setuprepair.New(projects, chats, identities, prompts, gate)
	return f
}
func (f *fixture) meta(t *testing.T) serviceproject.Meta {
	t.Helper()
	m, err := f.projects.Get(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAutomaticRepairRunsAgentAndVerifiesRealScript(t *testing.T) {
	f := newFixture(t)
	if err := f.worker.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := f.meta(t)
	if m.Status != serviceproject.StatusRunning || m.SetupFailed || m.SetupRepair.State != "succeeded" {
		t.Fatalf("repair result=%+v", m)
	}
	if f.setup.calls != 2 {
		t.Fatalf("setup ran %d times; want initial failure and independent verification", f.setup.calls)
	}
	if len(f.provider.requests) != 1 {
		t.Fatalf("agent attempts=%d", len(f.provider.requests))
	}
	r := f.provider.requests[0]
	if r.AccountID != "saved-account" || r.Model != "chosen-model" || r.Mode != "default" || r.Preferences.ApprovalPolicy != "on-request" || r.Preferences.SandboxPolicy != servicechat.NormalizeSandboxPolicy(f.source.SandboxPolicy) || r.ResumeID != "" {
		t.Fatalf("agent configuration not preserved: %+v", r)
	}
	if !strings.Contains(r.Prompt, "missing-installer.sh") || !strings.Contains(r.Prompt, "Do not hide failures") {
		t.Fatal("repair prompt missing diagnosis or repair boundaries")
	}
	if data, err := os.ReadFile(filepath.Join(f.project.Cwd, "restored-tool")); err != nil || string(data) != "restored" {
		t.Fatalf("tool was not restored: %q %v", data, err)
	}
	chat, err := f.chats.Get(context.Background(), servicechat.ID(m.SetupRepair.ChatID))
	if err != nil || !chat.SetupRepair || chat.Title != "Automatic setup repair" {
		t.Fatalf("missing visible repair chat: %+v %v", chat, err)
	}
	events, _ := f.chats.ReadEvents(context.Background(), chat.ID)
	if events[0].UserEmail != "member@example.test" {
		t.Fatal("repair lost originating actor")
	}
	// A new worker represents a backend restart. Successful repair stays done.
	worker := setuprepair.New(f.projects, f.chats, f.identities, f.prompts, f.gate)
	if err := worker.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.provider.requests) != 1 {
		t.Fatal("restart repeated completed repair")
	}
}

func TestFailedOrUnverifiedRepairsDoNotLoopAfterRestart(t *testing.T) {
	for _, agentFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(agentFailure), func(t *testing.T) {
			f := newFixture(t)
			f.provider.fail = agentFailure
			f.provider.leaveBroken = !agentFailure
			if err := f.worker.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			m := f.meta(t)
			if !m.NeedsSetupRepair() || m.SetupRepair.State != "failed" {
				t.Fatalf("failed repair hidden: %+v", m)
			}
			events, _ := f.chats.ReadEvents(context.Background(), servicechat.ID(m.SetupRepair.ChatID))
			for _, e := range events {
				if e.Type == "complete" && e.Status != "failed" {
					t.Fatal("reported success before setup verification")
				}
			}
			if !agentFailure && (len(f.usage.events) != 1 || len(f.usage.events[0].Usage) == 0) {
				t.Fatal("failed verification discarded the completed agent's usage")
			}
			worker := setuprepair.New(f.projects, f.chats, f.identities, f.prompts, f.gate)
			if err := worker.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(f.provider.requests) != 1 {
				t.Fatal("unsuccessful repair retried automatically")
			}
			// The failed repair must not lock users out of their agent.
			if _, err := f.projects.StartForAgent(context.Background(), m.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecoveryWaitsForMaintenanceAndEligibleActor(t *testing.T) {
	f := newFixture(t)
	f.gate.blocked = true
	if err := f.worker.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.meta(t).SetupRepair != nil || len(f.provider.requests) > 0 {
		t.Fatal("repair ran during update")
	}
	f.gate.blocked = false
	f.identities.registered = false
	if err := f.worker.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.meta(t).SetupRepair.State != "blocked" || len(f.provider.requests) > 0 {
		t.Fatal("repair used removed identity")
	}
	f.identities.registered = true
	f.identities.admin = false // no project membership
	if err := f.worker.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.provider.requests) > 0 {
		t.Fatal("repair used identity without project access")
	}
	f.identities.admin = true
	if err := f.worker.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.meta(t).SetupRepair.State != "succeeded" {
		t.Fatal("blocked recovery did not resume after valid identity")
	}
}

func TestInterruptedClaimIsNotAutomaticallyReissued(t *testing.T) {
	f := newFixture(t)
	claimed, err := f.projects.ClaimSetupRepair(context.Background(), f.project.ID, string(f.source.ID))
	if err != nil || !claimed {
		t.Fatalf("claim=%v %v", claimed, err)
	}
	if err := f.worker.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.provider.requests) > 0 || f.meta(t).SetupRepair.State != "failed" {
		t.Fatal("interrupted claim launched another run")
	}
}

func TestRepairReservesProjectUntilVerificationCompletes(t *testing.T) {
	f := newFixture(t)
	f.provider.entered = make(chan struct{})
	resume := make(chan struct{})
	f.provider.resume = resume
	done := make(chan error, 1)
	go func() { done <- f.worker.Reconcile(context.Background()) }()
	select {
	case <-f.provider.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("repair did not start")
	}
	_, err := f.prompts.Start(prompt.StartInput{ChatID: f.source.ID, Prompt: "concurrent work"}, nil)
	if !errors.Is(err, prompt.ErrSetupRepairRunning) {
		t.Errorf("interactive prompt overlapped repair: %v", err)
	}
	close(resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type hangingPrompts struct{}

func (hangingPrompts) ProjectBusy(serviceproject.ID) bool { return false }
func (hangingPrompts) Start(prompt.StartInput, func(servicechat.Event)) (prompt.RunHandle, error) {
	return prompt.RunHandle{Done: make(chan prompt.RunResult)}, nil
}

func TestRecoveryDeadlineBoundsAnUnresponsiveAgent(t *testing.T) {
	f := newFixture(t)
	synctest.Test(t, func(t *testing.T) {
		worker := setuprepair.New(f.projects, f.chats, f.identities, hangingPrompts{}, f.gate)
		started := time.Now()
		if err := worker.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		if time.Since(started) != 30*time.Minute {
			t.Fatalf("unexpected recovery deadline: %s", time.Since(started))
		}
		if f.meta(t).SetupRepair.State != "failed" {
			t.Fatal("timed-out recovery left running state")
		}
	})
}

type partiallyUnavailableProjects struct {
	*serviceproject.Service
	first serviceproject.Meta
	err   error
}

func (p partiallyUnavailableProjects) List(ctx context.Context) ([]serviceproject.Meta, error) {
	projects, err := p.Service.List(ctx)
	return append([]serviceproject.Meta{p.first}, projects...), err
}

func (p partiallyUnavailableProjects) ClaimSetupRepair(ctx context.Context, id serviceproject.ID, chatID string) (bool, error) {
	if id == p.first.ID {
		return false, p.err
	}
	return p.Service.ClaimSetupRepair(ctx, id, chatID)
}

func TestOneUnavailableContainerDoesNotBlockFleetRecovery(t *testing.T) {
	f := newFixture(t)
	unavailable := f.project
	unavailable.ID = "deadbeef"
	source, err := f.chats.Create(context.Background(), servicechat.Meta{ProjectID: servicechat.ProjectID(unavailable.ID), Provider: agent.ProviderCodex})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.chats.AppendEvent(context.Background(), source.ID, servicechat.Event{Type: "user", UserEmail: "member@example.test"}); err != nil {
		t.Fatal(err)
	}
	inspectionErr := errors.New("container inspection failed")
	projects := partiallyUnavailableProjects{Service: f.projects, first: unavailable, err: inspectionErr}
	worker := setuprepair.New(projects, f.chats, f.identities, f.prompts, f.gate)
	if err := worker.Reconcile(context.Background()); !errors.Is(err, inspectionErr) {
		t.Fatalf("inspection failure not reported: %v", err)
	}
	if f.meta(t).SetupRepair == nil || f.meta(t).SetupRepair.State != "succeeded" {
		t.Fatal("first container failure blocked later project recovery")
	}
}

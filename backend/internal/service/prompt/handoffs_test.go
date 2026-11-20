package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
	"github.com/futrx-com/remote.futrx.com/internal/model/conversation"
	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	"github.com/futrx-com/remote.futrx.com/internal/stores/sqlitechat"
)

type switchingProvider struct {
	id           harness.ProviderID
	requests     []harness.RunRequest
	histories    []conversation.History
	fail         bool
	interrupt    bool
	missing      bool
	noAcceptance bool
	nextSession  int
}

func (p *switchingProvider) ID() harness.ProviderID { return p.id }
func (p *switchingProvider) Capabilities(context.Context, harness.CapabilityRequest) (harness.Capabilities, error) {
	return harness.Capabilities{}, nil
}
func (p *switchingProvider) HistoryInput(h conversation.History, input string) (string, string, error) {
	p.histories = append(p.histories, h)
	return harness.PromptWithHistory(h, input)
}
func (p *switchingProvider) Run(ctx context.Context, req harness.RunRequest, emit func(harness.Event)) error {
	p.requests = append(p.requests, req)
	if p.missing && req.ResumeID != "" {
		p.missing = false
		return harness.ErrSessionNotFound
	}
	native := req.ResumeID
	if native == "" {
		p.nextSession++
		native = fmt.Sprintf("%s-native-%d", p.id, p.nextSession)
	}
	emit(harness.Event{Type: harness.EventSessionUpdated, SessionID: native})
	if !p.noAcceptance {
		emit(harness.Event{Type: harness.EventInputAccepted, SessionID: native, Status: "test_native_ack"})
	}
	emit(harness.Event{
		Type: harness.EventToolStarted, ItemID: "call-1", ToolName: "shell", Input: []byte(`{"command":"tests"}`),
		Raw: []byte(`{"item":{"id":"call-1","type":"commandExecution","status":"inProgress"}}`),
	})
	emit(harness.Event{
		Type: harness.EventToolCompleted, ItemID: "call-1", Output: string(p.id) + " tests passed",
		Raw: []byte(`{"item":{"id":"call-1","type":"commandExecution","status":"completed","aggregatedOutput":"tests passed","exitCode":0}}`),
	})
	emit(harness.Event{Type: harness.EventAssistantTextDelta, Text: string(p.id) + " reply"})
	if p.interrupt {
		emit(harness.Event{Type: harness.EventRunInterrupted})
		return nil
	}
	if p.fail {
		return errors.New("transport failed after output")
	}
	emit(harness.Event{Type: harness.EventRunCompleted})
	return nil
}

type handoffFixture struct {
	store         *sqlitechat.Store
	service       *Service
	claude, codex *switchingProvider
	id            chat.ID
}

func newHandoffFixture(t *testing.T) *handoffFixture {
	t.Helper()
	store, err := sqlitechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	meta, err := store.Create(context.Background(), chat.Meta{ID: "abcdef123456", Provider: harness.ProviderClaude, Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	claude, codex := &switchingProvider{id: harness.ProviderClaude}, &switchingProvider{id: harness.ProviderCodex}
	registry := harness.NewRegistry()
	if err = registry.Register(claude); err != nil {
		t.Fatal(err)
	}
	if err = registry.Register(codex); err != nil {
		t.Fatal(err)
	}
	return &handoffFixture{store: store, service: New(store, nil, nil, runhub.New(store), registry), claude: claude, codex: codex, id: meta.ID}
}
func (f *handoffFixture) run(t *testing.T, provider harness.ProviderID, account, prompt string) error {
	t.Helper()
	_, err := f.store.Update(context.Background(), f.id, func(m *chat.Meta) { m.Provider = provider; m.AccountID = account })
	if err != nil {
		t.Fatal(err)
	}
	return f.service.runPrompt(context.Background(), f.id, prompt, func(e ChatEvent) {
		if _, err := f.store.AppendEvent(context.Background(), f.id, e); err != nil {
			t.Fatal(err)
		}
	}, func(ChatEvent) {})
}

func TestHandoffClaudeCodexClaudeKeepsOnlyMissingHistory(t *testing.T) {
	f := newHandoffFixture(t)
	for _, step := range []struct {
		provider harness.ProviderID
		prompt   string
	}{{harness.ProviderClaude, "first request"}, {harness.ProviderCodex, "second request"}, {harness.ProviderClaude, "third request"}} {
		if err := f.run(t, step.provider, "", step.prompt); err != nil {
			t.Fatal(err)
		}
	}
	first, back := f.claude.requests[0], f.claude.requests[1]
	if first.ResumeID != "" || back.ResumeID != "claude-native-1" {
		t.Fatalf("resume: %q -> %q", first.ResumeID, back.ResumeID)
	}
	if strings.Contains(back.Prompt, "first request") || strings.Contains(back.Prompt, "claude reply") {
		t.Fatalf("already-covered history was replayed: %s", back.Prompt)
	}
	for _, want := range []string{"second request", "codex reply", "codex tests passed", `"toolCallId":"call-1"`, `"provider":"codex"`} {
		if !strings.Contains(back.Prompt, want) {
			t.Fatalf("missing %s from %s", want, back.Prompt)
		}
	}
	if strings.Count(back.Prompt, "third request") != 1 {
		t.Fatal(back.Prompt)
	}
	events, err := f.store.ReadEvents(context.Background(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	var users int
	for _, ev := range events {
		if ev.Type == "user" {
			users++
			if strings.Contains(ev.Text, "REMOTE_HISTORY") {
				t.Fatal("handoff became UI user message")
			}
		}
	}
	if users != 3 {
		t.Fatalf("user count=%d", users)
	}
	var raw []byte
	if err = f.store.DB().QueryRow(`SELECT sessions_json FROM chats WHERE id=?`, f.id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var sessions map[string]struct {
		State             string `json:"state"`
		HistoryThroughSeq int64  `json:"historyThroughSeq"`
	}
	if err = json.Unmarshal(raw, &sessions); err != nil {
		t.Fatal(err)
	}
	session := sessions[`["claude",""]`]
	if session.State != "ready" || session.HistoryThroughSeq != events[len(events)-1].Seq {
		t.Fatalf("session=%+v, tail=%d", session, events[len(events)-1].Seq)
	}
}

func TestHandoffUncertainRunsNeverResumeSameNativeSession(t *testing.T) {
	for _, failure := range []string{"transport", "interruption", "no-acceptance"} {
		t.Run(failure, func(t *testing.T) {
			f := newHandoffFixture(t)
			f.claude.fail = failure == "transport"
			f.claude.interrupt = failure == "interruption"
			f.claude.noAcceptance = failure == "no-acceptance"
			_ = f.run(t, harness.ProviderClaude, "", "first request")
			f.claude.fail = false
			f.claude.interrupt = false
			f.claude.noAcceptance = false
			if err := f.run(t, harness.ProviderClaude, "", "try again"); err != nil {
				t.Fatal(err)
			}
			req := f.claude.requests[1]
			if req.ResumeID != "" {
				t.Fatalf("unsafe resume=%q", req.ResumeID)
			}
			if !strings.Contains(req.Prompt, "first request") || !strings.Contains(req.Prompt, "claude reply") {
				t.Fatal("fresh session did not receive retained history")
			}
		})
	}
}

func TestHandoffMissingNativeSessionRetriesWithFullHistory(t *testing.T) {
	f := newHandoffFixture(t)
	if err := f.run(t, harness.ProviderClaude, "", "old question"); err != nil {
		t.Fatal(err)
	}
	f.claude.missing = true
	if err := f.run(t, harness.ProviderClaude, "", "new question"); err != nil {
		t.Fatal(err)
	}
	if len(f.claude.requests) != 3 {
		t.Fatal(len(f.claude.requests))
	}
	retry := f.claude.requests[2]
	if retry.ResumeID != "" || !strings.Contains(retry.Prompt, "old question") {
		t.Fatal(retry)
	}
	if strings.Count(retry.Prompt, "new question") != 1 {
		t.Fatal("current question replayed as history")
	}
	events, err := f.store.ReadEvents(context.Background(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	users := 0
	for _, ev := range events {
		if ev.Type == "user" {
			users++
		}
	}
	if users != 2 {
		t.Fatal(users)
	}
}

func TestHandoffAccountsHaveIndependentNativeSessions(t *testing.T) {
	f := newHandoffFixture(t)
	for _, step := range []struct{ account, prompt string }{{"account-a", "first"}, {"account-b", "second"}, {"account-a", "third"}} {
		if err := f.run(t, harness.ProviderClaude, step.account, step.prompt); err != nil {
			t.Fatal(err)
		}
	}
	if f.claude.requests[1].ResumeID != "" {
		t.Fatal("second account resumed first account")
	}
	if f.claude.requests[2].ResumeID != "claude-native-1" {
		t.Fatal("original account mapping lost")
	}
	if !strings.Contains(f.claude.requests[2].Prompt, "second") {
		t.Fatal("other account conversation missing")
	}
}

func TestHandoffAfterRewindUsesRetainedBranchOnly(t *testing.T) {
	f := newHandoffFixture(t)
	if err := f.run(t, harness.ProviderClaude, "", "retained question"); err != nil {
		t.Fatal(err)
	}
	if err := f.run(t, harness.ProviderClaude, "", "discarded question"); err != nil {
		t.Fatal(err)
	}
	events, err := f.store.ReadEvents(context.Background(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	var cutoff int64
	for _, event := range events {
		if event.Type == "user" && event.Text == "discarded question" {
			cutoff = event.T
		}
	}
	if cutoff == 0 {
		t.Fatal("missing rewind boundary")
	}
	if err = f.store.RewindBranch(context.Background(), f.id, cutoff); err != nil {
		t.Fatal(err)
	}
	if err = f.run(t, harness.ProviderClaude, "", "replacement question"); err != nil {
		t.Fatal(err)
	}
	req := f.claude.requests[2]
	if req.ResumeID != "" {
		t.Fatal("rewind resumed native future")
	}
	if !strings.Contains(req.Prompt, "retained question") || strings.Contains(req.Prompt, "discarded question") {
		t.Fatal(req.Prompt)
	}
}

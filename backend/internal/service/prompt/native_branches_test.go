package prompt

import (
	"context"
	"strings"
	"testing"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

func forkHandoffFixture(t *testing.T, f *handoffFixture) chat.ID {
	t.Helper()
	source := f.id
	_, err := f.store.ForkChat(context.Background(), source, chat.Meta{
		ID: "fedcba654321", Provider: harness.ProviderClaude, Cwd: t.TempDir(),
	}, -1)
	if err != nil {
		t.Fatal(err)
	}
	f.id = "fedcba654321"
	return source
}

func TestNativeRewindUsesRetainedCheckpointAndMissingHarnessHistory(t *testing.T) {
	f := newHandoffFixture(t)
	f.claude.nativeCheckpoints = true
	for _, step := range []struct {
		provider harness.ProviderID
		text     string
	}{{harness.ProviderClaude, "retained Claude turn"}, {harness.ProviderCodex, "retained Codex turn"}, {harness.ProviderClaude, "discarded Claude turn"}} {
		if err := f.run(t, step.provider, "account-a", step.text); err != nil {
			t.Fatal(err)
		}
	}
	events, err := f.store.ReadEvents(context.Background(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	var before int64
	for _, event := range events {
		if event.Type == "user" && event.Text == "discarded Claude turn" {
			before = event.T
		}
	}
	if err = f.store.RewindBranch(context.Background(), f.id, before); err != nil {
		t.Fatal(err)
	}
	if err = f.run(t, harness.ProviderClaude, "account-a", "replacement turn"); err != nil {
		t.Fatal(err)
	}
	req := f.claude.requests[2]
	if !req.Fork || req.NativeCheckpoint != "claude-turn-1" || req.ResumeID != "claude-native-1" || req.NativeConversationID != string(f.id) {
		t.Fatalf("wrong native rewind: %+v", req)
	}
	if strings.Contains(req.Prompt, "retained Claude turn") || strings.Contains(req.Prompt, "discarded Claude turn") || !strings.Contains(req.Prompt, "retained Codex turn") || strings.Count(req.Prompt, "replacement turn") != 1 {
		t.Fatalf("wrong suffix: %s", req.Prompt)
	}
	if err = f.run(t, harness.ProviderClaude, "account-a", "continue branch"); err != nil {
		t.Fatal(err)
	}
	resumed := f.claude.requests[3]
	if resumed.Fork || resumed.NativeCheckpoint != "" || resumed.ResumeID != "claude-native-2" || strings.Contains(resumed.Prompt, "retained Codex turn") {
		t.Fatalf("new native branch was not resumed: %+v", resumed)
	}
}

func TestNativeForkKeepsCapturedBoundaryAndSourceAccountHome(t *testing.T) {
	f := newHandoffFixture(t)
	f.claude.nativeCheckpoints = true
	if err := f.run(t, harness.ProviderClaude, "account-a", "shared prefix"); err != nil {
		t.Fatal(err)
	}
	source := forkHandoffFixture(t, f)
	target := f.id
	f.id = source
	if err := f.run(t, harness.ProviderClaude, "account-a", "source future"); err != nil {
		t.Fatal(err)
	}
	f.id = target
	if err := f.run(t, harness.ProviderClaude, "account-a", "fork request"); err != nil {
		t.Fatal(err)
	}
	req := f.claude.requests[2]
	if !req.Fork || req.NativeCheckpoint != "claude-turn-1" || req.ResumeID != "claude-native-1" || req.ConversationID != string(target) || req.NativeConversationID != string(source) {
		t.Fatalf("wrong captured branch: %+v", req)
	}
	if strings.Contains(req.Prompt, "shared prefix") || strings.Contains(req.Prompt, "source future") {
		t.Fatal(req.Prompt)
	}
	if err := f.run(t, harness.ProviderClaude, "account-a", "fork continuation"); err != nil {
		t.Fatal(err)
	}
	req = f.claude.requests[3]
	if req.ResumeID != "claude-native-2" || req.Fork || req.NativeConversationID != string(source) {
		t.Fatalf("native fork home lost: %+v", req)
	}
	f.id = source
	if err := f.run(t, harness.ProviderClaude, "account-a", "source continuation"); err != nil {
		t.Fatal(err)
	}
	if req = f.claude.requests[4]; req.ResumeID != "claude-native-1" || req.Fork {
		t.Fatalf("source mapping changed: %+v", req)
	}
}

func TestNativeForkUnavailableReconstructsOnceWithAllRetainedHistory(t *testing.T) {
	f := newHandoffFixture(t)
	f.claude.nativeCheckpoints = true
	if err := f.run(t, harness.ProviderClaude, "account-a", "retained prefix"); err != nil {
		t.Fatal(err)
	}
	forkHandoffFixture(t, f)
	f.claude.unavailableCheckpoint = true
	if err := f.run(t, harness.ProviderClaude, "account-a", "current question"); err != nil {
		t.Fatal(err)
	}
	if len(f.claude.requests) != 3 {
		t.Fatalf("attempts=%d", len(f.claude.requests))
	}
	failed, retry := f.claude.requests[1], f.claude.requests[2]
	if !failed.Fork || failed.NativeCheckpoint == "" {
		t.Fatal("did not try native fork")
	}
	if retry.Fork || retry.ResumeID != "" || retry.NativeCheckpoint != "" || retry.NativeConversationID != "" {
		t.Fatalf("native options survived recovery: %+v", retry)
	}
	if !strings.Contains(retry.Prompt, "retained prefix") || strings.Count(retry.Prompt, "current question") != 1 {
		t.Fatal(retry.Prompt)
	}
	if err := f.run(t, harness.ProviderClaude, "account-a", "continue fresh session"); err != nil {
		t.Fatal(err)
	}
	if req := f.claude.requests[3]; req.NativeConversationID != string(f.id) || req.Fork || req.ResumeID != "claude-native-2" {
		t.Fatalf("fresh home was not persisted: %+v", req)
	}
}

func TestNativeRecoveryNeverRetriesAcceptedInput(t *testing.T) {
	for _, recoveryError := range []error{harness.ErrNativeCheckpointUnavailable, harness.ErrSessionNotFound} {
		t.Run(recoveryError.Error(), func(t *testing.T) {
			f := newHandoffFixture(t)
			f.claude.nativeCheckpoints = true
			if err := f.run(t, harness.ProviderClaude, "", "retained prefix"); err != nil {
				t.Fatal(err)
			}
			forkHandoffFixture(t, f)
			f.claude.errorAfterInput = recoveryError
			if err := f.run(t, harness.ProviderClaude, "", "accepted request"); err == nil {
				t.Fatal("expected failure")
			}
			if len(f.claude.requests) != 2 {
				t.Fatal("accepted input was retried")
			}
			f.claude.errorAfterInput = nil
			if err := f.run(t, harness.ProviderClaude, "", "user retry"); err != nil {
				t.Fatal(err)
			}
			if req := f.claude.requests[2]; req.ResumeID != "" || req.Fork || !strings.Contains(req.Prompt, "accepted request") {
				t.Fatalf("uncertain native session reused: %+v", req)
			}
		})
	}
}

func TestNativeForkRequiresNewSessionIdentity(t *testing.T) {
	f := newHandoffFixture(t)
	f.claude.nativeCheckpoints = true
	if err := f.run(t, harness.ProviderClaude, "", "retained prefix"); err != nil {
		t.Fatal(err)
	}
	forkHandoffFixture(t, f)
	f.claude.reuseForkSession = true
	if err := f.run(t, harness.ProviderClaude, "", "branch request"); err == nil || !strings.Contains(err.Error(), "new session") {
		t.Fatalf("fork reused source: %v", err)
	}
	if len(f.claude.requests) != 2 {
		t.Fatal("invalid native fork retried")
	}
}

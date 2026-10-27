package kimicodex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type accountKeys struct{}

func (accountKeys) APIKey() (string, bool) { return "default-key", true }
func (accountKeys) APIKeyFor(id string) (string, bool) {
	return "selected-key", id == "selected-account"
}

type accountModels struct{ t *testing.T }

func (s accountModels) Models(_ context.Context, key string) ([]modelInfo, error) {
	if key != "selected-key" {
		s.t.Errorf("discovery used the wrong account")
	}
	return parseModels([]byte(catalogFixture))
}

func TestUnknownAccountDoesNotFallBackToActiveAccount(t *testing.T) {
	p := &Provider{apiKeys: accountKeys{}}
	if _, err := p.apiKey("missing"); !errors.Is(err, ErrKimiCodexAPIKeyMissing) {
		t.Fatalf("unknown account = %v", err)
	}
}

func TestProviderRunsSelectedAccountWithCodexSessionsAndToolEvents(t *testing.T) {
	for _, method := range []string{"thread/start", "thread/resume", "thread/fork"} {
		t.Run(method, func(t *testing.T) {
			dir := t.TempDir()
			script := `#!/bin/sh
case "$*" in *'KIMI_API_KEY=selected-key'*) ;; *) exit 2 ;; esac
case "$*" in *'CODEX_HOME=/root/.kimi-codex'*) ;; *) exit 3 ;; esac
while IFS= read -r line; do
 case "$line" in
  *'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
  *'"id":2'*)
   case "$line" in *'EXPECTED_METHOD'*) ;; *) exit 4 ;; esac
   printf '%s\n' '{"id":2,"result":{"thread":{"id":"kimi-thread"},"model":"k3"}}' ;;
  *'"id":3'*)
   case "$line" in *'"effort":"max"'*) ;; *) exit 5 ;; esac
   case "$line" in *'"mode":"plan"'*) ;; *) exit 6 ;; esac
   case "$line" in *'"serviceTier"'*) exit 7 ;; esac
   printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1","status":"inProgress","items":[]}}}'
   printf '%s\n' '{"method":"item/started","params":{"threadId":"kimi-thread","turnId":"turn-1","item":{"id":"tool-1","type":"commandExecution","command":"pwd"}}}'
   printf '%s\n' '{"method":"item/completed","params":{"threadId":"kimi-thread","turnId":"turn-1","item":{"id":"tool-1","type":"commandExecution","command":"pwd","status":"completed","aggregatedOutput":"/workspace"}}}'
   printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"kimi-thread","turnId":"turn-1","itemId":"message-1","delta":"Kimi reply"}}'
   printf '%s\n' '{"method":"turn/completed","params":{"threadId":"kimi-thread","turn":{"id":"turn-1","status":"completed","items":[]}}}'
   exit 0 ;;
 esac
done
`
			script = strings.ReplaceAll(script, "EXPECTED_METHOD", method)
			if err := os.WriteFile(filepath.Join(dir, "lxc"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			assets := &kimiCodexTestRuntimeAssets{}
			p := newProvider(kimiCodexTestPreparer{project: agent.PreparedProject{ContainerName: "project-container"}}, accountKeys{}, accountModels{t}, assets, "codex")
			req := agent.RunRequest{ProjectID: "project", ConversationID: "chat", AccountID: "selected-account", Model: "k3", Prompt: "plan", Mode: agent.RunModePlan,
				Preferences: agent.RunPreferences{ReasoningEffort: "max", ServiceTier: "fast"}}
			if method != "thread/start" {
				req.ResumeID = "kimi-thread"
			}
			req.Fork = method == "thread/fork"
			if req.Fork {
				req.ResumeID = "parent-thread"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var events []agent.Event
			if err := p.Run(ctx, req, func(e agent.Event) { events = append(events, e) }); err != nil {
				t.Fatal(err)
			}
			seen := map[agent.EventType]bool{}
			for _, e := range events {
				if e.Provider != agent.ProviderKimiCodex {
					t.Fatalf("event misattributed: %#v", e)
				}
				seen[e.Type] = true
			}
			if method != "thread/resume" && !seen[agent.EventSessionUpdated] {
				t.Fatal("new/forked thread did not publish its session")
			}
			for _, typ := range []agent.EventType{agent.EventToolStarted, agent.EventToolCompleted, agent.EventAssistantTextDelta, agent.EventRunCompleted} {
				if !seen[typ] {
					t.Fatalf("missing %s", typ)
				}
			}
		})
	}
}

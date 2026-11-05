package kimi

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func isolatedNativeCLI(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	if os.Getenv("REMOTE_KIMI_SMOKE") != "1" {
		t.Skip("set REMOTE_KIMI_SMOKE=1 for pinned CLI compatibility tests")
	}
	binary, err := exec.LookPath("kimi")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(func() { server.CloseClientConnections(); server.Close() })
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "kimi"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "kimi", "config.toml"), []byte("telemetry = false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REMOTE_KIMI_SMOKE_BINARY", binary)
	t.Setenv("REMOTE_KIMI_SMOKE_PATH", os.Getenv("PATH"))
	t.Setenv("REMOTE_KIMI_SMOKE_HOME", home)
	t.Setenv("REMOTE_KIMI_SMOKE_URL", server.URL+"/v1")
	fakeCLI(t, `exec /usr/bin/env -i PATH="$REMOTE_KIMI_SMOKE_PATH" HOME="$REMOTE_KIMI_SMOKE_HOME" KIMI_CODE_HOME="$REMOTE_KIMI_SMOKE_HOME/kimi" NO_COLOR=1 KIMI_MODEL_NAME=smoke-model KIMI_MODEL_API_KEY=dummy-local-key KIMI_MODEL_BASE_URL="$REMOTE_KIMI_SMOKE_URL" KIMI_MODEL_PROVIDER_TYPE=openai KIMI_MODEL_CAPABILITIES=text_in,thinking KIMI_CODE_EXPERIMENTAL_SECONDARY_MODEL=0 "$REMOTE_KIMI_SMOKE_BINARY" "$@"`)
	return home
}
func streamNative(w http.ResponseWriter, delta map[string]any, finish string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, chunk := range []map[string]any{
		{"id": "test", "object": "chat.completion.chunk", "model": "smoke-model", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}},
		{"id": "test", "object": "chat.completion.chunk", "model": "smoke-model", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 3, "prompt_tokens_details": map[string]int{"cached_tokens": 2}}},
	} {
		raw, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", raw)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}
func nativeTool(w http.ResponseWriter, name string, args any) {
	raw, _ := json.Marshal(args)
	streamNative(w, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "call-1", "type": "function", "function": map[string]any{"name": name, "arguments": string(raw)}}}}, "tool_calls")
}

func TestInstalledNativeQuestion(t *testing.T) {
	var requests atomic.Int32
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, req *http.Request) {
		if requests.Add(1) == 1 {
			nativeTool(w, "AskUserQuestion", map[string]any{"questions": []any{map[string]any{"question": "Choose components", "header": "Scope", "multi_select": true, "options": []any{map[string]any{"label": "Frontend", "description": "UI"}, map[string]any{"label": "Backend", "description": "API"}}}}})
			return
		}
		streamNative(w, map[string]any{"content": "ANSWERED", "reasoning_content": "Question resolved."}, "stop")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	responses := make(chan agent.InteractionResponse, 4)
	var question, resolved, complete bool
	var diagnostic, thinking string
	err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: "Ask which components to change.", Preferences: agent.RunPreferences{ApprovalPolicy: "untrusted"}, InteractionResponses: responses}, func(ev agent.Event) {
		switch ev.Type {
		case agent.EventInteractionRequest:
			if ev.Status == "approval" {
				responses <- agent.InteractionResponse{ID: ev.InteractionID, Result: json.RawMessage(`{"decision":"accept"}`)}
				return
			}
			question = true
			var input struct {
				Questions []struct {
					ID          string `json:"id"`
					MultiSelect bool   `json:"multiSelect"`
					Options     []struct {
						ID string `json:"id"`
					} `json:"options"`
				} `json:"questions"`
			}
			if err := json.Unmarshal(ev.Input, &input); err != nil || len(input.Questions) != 1 {
				t.Errorf("question input: %s", ev.Input)
				return
			}
			q := input.Questions[0]
			if !q.MultiSelect {
				t.Error("multi-select lost")
			}
			answer, _ := json.Marshal(map[string]any{"answers": map[string]any{q.ID: map[string]any{"answers": []string{q.Options[0].ID, q.Options[1].ID}}}})
			responses <- agent.InteractionResponse{ID: ev.InteractionID, Result: answer}
		case agent.EventInteractionDone:
			resolved = true
		case agent.EventRunCompleted:
			complete = true
		case agent.EventRunFailed:
			diagnostic = ev.Message
		case agent.EventReasoningDelta:
			thinking += ev.Text
		case agent.EventToolCompleted:
			if ev.IsError {
				t.Errorf("tool failed: %s", ev.Output)
			}
		}
	})
	if err != nil || !question || !resolved || !complete || thinking == "" {
		t.Fatalf("err=%v question=%t resolved=%t complete=%t thinking=%q diagnostic=%q", err, question, resolved, complete, thinking, diagnostic)
	}
}

func TestInstalledNativeDelegation(t *testing.T) {
	for _, background := range []bool{false, true} {
		t.Run(fmt.Sprint(background), func(t *testing.T) {
			var requests atomic.Int32
			home := isolatedNativeCLI(t, func(w http.ResponseWriter, req *http.Request) {
				var body struct {
					Messages []struct {
						Role    string          `json:"role"`
						Content json.RawMessage `json:"content"`
					} `json:"messages"`
				}
				_ = json.NewDecoder(req.Body).Decode(&body)
				child := false
				for _, m := range body.Messages {
					if m.Role == "user" && strings.Contains(string(m.Content), "CHILD_TASK_ONLY") {
						child = true
					}
				}
				if child {
					time.Sleep(700 * time.Millisecond)
					streamNative(w, map[string]any{"content": "CHILD_DONE", "reasoning_content": "Child reasoning."}, "stop")
					return
				}
				if requests.Add(1) == 1 {
					nativeTool(w, "Agent", map[string]any{"subagent_type": "coder", "description": "Check child lifecycle", "prompt": "CHILD_TASK_ONLY", "run_in_background": background})
					return
				}
				streamNative(w, map[string]any{"content": "MAIN_DONE"}, "stop")
			})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var children = map[string]string{}
			var complete bool
			var diagnostic string
			var usage agent.Usage
			err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: "Delegate a check.", Preferences: agent.RunPreferences{ApprovalPolicy: "never"}}, func(ev agent.Event) {
				switch ev.Type {
				case agent.EventCollaboration:
					children[ev.ItemID] = ev.Status
				case agent.EventRunCompleted:
					complete = true
					usage, _ = agent.ParseUsage(ev.Usage)
				case agent.EventRunFailed:
					diagnostic = ev.Message
				case agent.EventToolCompleted:
					if ev.IsError {
						t.Errorf("tool failed: %s", ev.Output)
					}
				}
			})
			if err != nil || !complete || len(children) != 1 {
				t.Fatalf("err=%v complete=%t children=%v usage=%+v diagnostic=%q", err, complete, children, usage, diagnostic)
			}
			for _, status := range children {
				if status != "completed" {
					t.Errorf("child status=%s", status)
				}
			}
			if usage.InputTokens < 24 || usage.CacheReadTokens < 6 || usage.OutputTokens < 9 {
				t.Errorf("child usage missing: %+v", usage)
			}
		})
	}
}

func TestInstalledNativeResumeForkAndPlan(t *testing.T) {
	var requests atomic.Int32
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		streamNative(w, map[string]any{"content": "TURN_DONE", "reasoning_content": "Native reasoning."}, "stop")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session := ""
	for i, req := range []agent.RunRequest{
		{Prompt: "First turn", Mode: agent.RunModePlan},
		{Prompt: "Continue"},
		{Prompt: "Fork this context", Fork: true},
	} {
		req.Cwd = home
		req.ResumeID = session
		var gotSession, diagnostic string
		var complete bool
		var usage agent.Usage
		err := (&Provider{}).Run(ctx, req, func(ev agent.Event) {
			switch ev.Type {
			case agent.EventSessionUpdated:
				gotSession = ev.SessionID
			case agent.EventRunFailed:
				diagnostic = ev.Message
			case agent.EventRunCompleted:
				complete = true
				usage, _ = agent.ParseUsage(ev.Usage)
			}
		})
		if err != nil || !complete || gotSession == "" {
			t.Fatalf("run %d: err=%v complete=%t diagnostic=%s", i, err, complete, diagnostic)
		}
		if i == 1 && gotSession != session {
			t.Error("resume created a different session")
		}
		if i == 2 && gotSession == session {
			t.Error("fork reused parent session")
		}
		if usage.TotalTokens() != 13 {
			t.Errorf("resumed history counted as new usage: %+v", usage)
		}
		session = gotSession
	}
	if requests.Load() != 3 {
		t.Fatalf("requests=%d", requests.Load())
	}
}

func TestInstalledNativeCancellationStopsBackgroundChild(t *testing.T) {
	var first atomic.Bool
	var disconnected atomic.Bool
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		if first.CompareAndSwap(false, true) {
			nativeTool(w, "Agent", map[string]any{"subagent_type": "coder", "description": "Cancellation test", "prompt": "Wait for cancellation", "run_in_background": true})
			return
		}
		<-req.Context().Done()
		disconnected.Store(true)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var complete, interrupted bool
	err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: "Delegate a check.", Preferences: agent.RunPreferences{ApprovalPolicy: "never"}}, func(ev agent.Event) {
		if ev.Type == agent.EventCollaboration && ev.Status == "running" {
			time.AfterFunc(300*time.Millisecond, cancel)
		}
		if ev.Type == agent.EventRunCompleted {
			complete = true
		}
		if ev.Type == agent.EventRunInterrupted {
			interrupted = true
		}
	})
	if err != nil || complete || !interrupted {
		t.Fatalf("err=%v complete=%t interrupted=%t", err, complete, interrupted)
	}
	// The mock model's active requests must be closed by native abort/shutdown.
	if !disconnected.Load() {
		t.Error("child model connection was not cancelled")
	}
}

func TestInstalledNativeSwarm(t *testing.T) {
	var mainRequests atomic.Int32
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		for _, m := range body.Messages {
			if m.Role == "user" && strings.Contains(string(m.Content), "SWARM_CHILD_") {
				streamNative(w, map[string]any{"content": "SWARM_REPORT"}, "stop")
				return
			}
		}
		if mainRequests.Add(1) == 1 {
			nativeTool(w, "AgentSwarm", map[string]any{"description": "Two independent checks", "items": []string{"a", "b"}, "prompt_template": "SWARM_CHILD_{{item}}", "subagent_type": "coder"})
			return
		}
		streamNative(w, map[string]any{"content": "SWARM_DONE"}, "stop")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	children := map[string]string{}
	var complete bool
	var diagnostic string
	err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: "/swarm on\nRun two checks.", Preferences: agent.RunPreferences{ApprovalPolicy: "never"}}, func(ev agent.Event) {
		switch ev.Type {
		case agent.EventCollaboration:
			children[ev.ItemID] = ev.Status
		case agent.EventRunCompleted:
			complete = true
		case agent.EventRunFailed:
			diagnostic = ev.Message
		case agent.EventToolCompleted:
			if ev.IsError {
				t.Errorf("tool error: %s", ev.Output)
			}
		}
	})
	if err != nil || !complete || len(children) != 2 {
		t.Fatalf("err=%v complete=%t children=%v diagnostic=%s", err, complete, children, diagnostic)
	}
	for _, status := range children {
		if status != "completed" {
			t.Errorf("status=%s", status)
		}
	}
}

func TestInstalledNativeCapabilities(t *testing.T) {
	isolatedNativeCLI(t, func(w http.ResponseWriter, r *http.Request) { t.Error("capability discovery sent a model request") })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	caps, err := (&Provider{}).Capabilities(ctx, agent.CapabilityRequest{})
	if err != nil || len(caps.Models) < 2 || len(caps.Modes) != 2 || len(caps.ApprovalPolicies) != 3 {
		t.Fatalf("caps=%+v err=%v", caps, err)
	}
	found := false
	for _, m := range caps.Models {
		if m.ID == "__kimi_env_model__" {
			found = true
		}
	}
	if !found {
		t.Fatal("environment model missing from native catalog")
	}
}

//go:embed testdata/mcp.mjs
var smokeMCP string

func TestInstalledNativeBrowserMCP(t *testing.T) {
	var requests atomic.Int32
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			nativeTool(w, "mcp__remote_browser__browser_snapshot", map[string]any{})
			return
		}
		streamNative(w, map[string]any{"content": "BROWSER_DONE"}, "stop")
	})
	bin := t.TempDir()
	script := filepath.Join(bin, "mcp.mjs")
	if err := os.WriteFile(script, []byte(smokeMCP), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "npx"), []byte("#!/bin/sh\nexec /usr/bin/node '"+strings.ReplaceAll(script, "'", "'\"'\"'")+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REMOTE_KIMI_SMOKE_PATH", bin+string(os.PathListSeparator)+os.Getenv("REMOTE_KIMI_SMOKE_PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var complete, tool bool
	var diagnostic string
	err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: "Inspect the browser.", EnableBrowser: true, Preferences: agent.RunPreferences{ApprovalPolicy: "never"}}, func(ev agent.Event) {
		switch ev.Type {
		case agent.EventRunCompleted:
			complete = true
		case agent.EventRunFailed:
			diagnostic = ev.Message
		case agent.EventToolCompleted:
			if ev.IsError {
				t.Errorf("tool: %s", ev.Output)
			}
			if strings.Contains(ev.Output, "BROWSER_FIXTURE_OK") {
				tool = true
			}
		}
	})
	if err != nil || !complete || !tool {
		t.Fatalf("err=%v complete=%t tool=%t diagnostic=%s", err, complete, tool, diagnostic)
	}
}

func TestInstalledNativeCustomAgentAndSkill(t *testing.T) {
	var sawProfile, sawSkill atomic.Bool
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		if strings.Contains(string(raw), "CUSTOM_PROFILE_MARKER") {
			sawProfile.Store(true)
		}
		if strings.Contains(string(raw), "CUSTOM_SKILL_MARKER") {
			sawSkill.Store(true)
		}
		streamNative(w, map[string]any{"content": "CUSTOM_DONE"}, "stop")
	})
	for name, content := range map[string]string{
		"agents/custom-review.md":      "---\nname: custom-review\ndescription: Custom fixture agent\ntools: []\n---\nCUSTOM_PROFILE_MARKER",
		"skills/custom-skill/SKILL.md": "---\nname: custom-skill\ndescription: Custom fixture skill\n---\nCUSTOM_SKILL_MARKER",
	} {
		path := filepath.Join(home, "kimi", name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, prompt := range []string{"/agent custom-review\nReview this.", "/skill:custom-skill test arguments"} {
		var diagnostic string
		err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: prompt}, func(ev agent.Event) {
			if ev.Type == agent.EventRunFailed {
				diagnostic = ev.Message
			}
		})
		if err != nil {
			t.Fatalf("err=%v diagnostic=%s", err, diagnostic)
		}
	}
	if !sawProfile.Load() || !sawSkill.Load() {
		t.Fatalf("profile=%t skill=%t", sawProfile.Load(), sawSkill.Load())
	}
}

func TestInstalledNativeStopIndividualAgent(t *testing.T) {
	var mainRequests atomic.Int32
	var childCancelled atomic.Bool
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		for _, m := range body.Messages {
			if m.Role == "user" && strings.Contains(string(m.Content), "STOP_CHILD_TEST") {
				<-req.Context().Done()
				childCancelled.Store(true)
				return
			}
		}
		if mainRequests.Add(1) == 1 {
			nativeTool(w, "Agent", map[string]any{"subagent_type": "coder", "description": "Stop just this agent", "prompt": "STOP_CHILD_TEST", "run_in_background": true})
			return
		}
		streamNative(w, map[string]any{"content": "MAIN_FINISHED"}, "stop")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	responses := make(chan agent.InteractionResponse, 2)
	var sent, complete bool
	var diagnostic string
	err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: "Delegate a check.", Preferences: agent.RunPreferences{ApprovalPolicy: "never"}, InteractionResponses: responses}, func(ev agent.Event) {
		if ev.Type == agent.EventCollaboration && !sent {
			var data struct {
				ID string `json:"stopInteractionId"`
			}
			_ = json.Unmarshal(ev.Data, &data)
			if data.ID != "" {
				sent = true
				time.AfterFunc(300*time.Millisecond, func() {
					responses <- agent.InteractionResponse{ID: data.ID, Result: json.RawMessage(`{"action":"cancel"}`)}
				})
			}
		}
		if ev.Type == agent.EventRunCompleted {
			complete = true
		}
		if ev.Type == agent.EventRunFailed {
			diagnostic = ev.Message
		}
	})
	if err != nil || !complete || !sent || !childCancelled.Load() {
		t.Fatalf("err=%v complete=%t sent=%t cancelled=%t diagnostic=%s", err, complete, sent, childCancelled.Load(), diagnostic)
	}
}

func TestInstalledNativeGoal(t *testing.T) {
	var requests atomic.Int32
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			nativeTool(w, "UpdateGoal", map[string]any{"status": "complete"})
			return
		}
		streamNative(w, map[string]any{"content": "GOAL_DONE"}, "stop")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var complete, goal bool
	var diagnostic string
	err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: "/goal Return a short status", Preferences: agent.RunPreferences{ApprovalPolicy: "never"}}, func(ev agent.Event) {
		if ev.Type == agent.EventRunCompleted {
			complete = true
		}
		if ev.Type == agent.EventRunFailed {
			diagnostic = ev.Message
		}
		if ev.Native != nil && ev.Native.Method == "kimi/goal.updated" {
			goal = true
		}
		if ev.Type == agent.EventToolCompleted && ev.IsError {
			t.Errorf("goal tool: %s", ev.Output)
		}
	})
	if err != nil || !complete || !goal {
		t.Fatalf("err=%v complete=%t goal=%t diagnostic=%s", err, complete, goal, diagnostic)
	}
}

func TestInstalledNativeApproval(t *testing.T) {
	var requests atomic.Int32
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			nativeTool(w, "Bash", map[string]any{"command": "printf APPROVAL_OK", "description": "Approval fixture"})
			return
		}
		streamNative(w, map[string]any{"content": "APPROVED_DONE"}, "stop")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	responses := make(chan agent.InteractionResponse, 2)
	var asked, resolved, complete bool
	var diagnostic string
	err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: "Run the fixture.", Preferences: agent.RunPreferences{ApprovalPolicy: "untrusted"}, InteractionResponses: responses}, func(ev agent.Event) {
		switch ev.Type {
		case agent.EventInteractionRequest:
			asked = true
			responses <- agent.InteractionResponse{ID: ev.InteractionID, Result: json.RawMessage(`{"decision":"accept"}`)}
		case agent.EventInteractionDone:
			resolved = true
		case agent.EventRunCompleted:
			complete = true
		case agent.EventRunFailed:
			diagnostic = ev.Message
		case agent.EventToolCompleted:
			if ev.IsError {
				t.Errorf("tool=%s", ev.Output)
			}
		}
	})
	if err != nil || !asked || !resolved || !complete {
		t.Fatalf("err=%v asked=%t resolved=%t complete=%t diagnostic=%s", err, asked, resolved, complete, diagnostic)
	}
}

func TestInstalledNativePlanAlternativeApproval(t *testing.T) {
	var requests atomic.Int32
	var selected atomic.Bool
	planPath := regexp.MustCompile(`/[^\s"<>]+/agents/main/plans/[^\s"<>]+\.md`)
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		if requests.Add(1) == 1 {
			// The real CLI creates the plan path and sends it to the model.
			// Write only the isolated fixture's plan, then request native review.
			path := strings.TrimSuffix(planPath.FindString(string(raw)), `\`)
			if path == "" {
				t.Error("native plan path was not supplied to the model")
				streamNative(w, map[string]any{"content": "No plan path"}, "stop")
				return
			}
			if err := os.WriteFile(path, []byte("Choose approach A or approach B."), 0600); err != nil {
				t.Error(err)
			}
			nativeTool(w, "ExitPlanMode", map[string]any{"options": []any{
				map[string]any{"label": "Approach A", "description": "First approach"},
				map[string]any{"label": "Approach B", "description": "Second approach"},
			}})
			return
		}
		selected.Store(strings.Contains(string(raw), "Selected approach: Approach B"))
		streamNative(w, map[string]any{"content": "PLAN_APPROVED"}, "stop")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	responses := make(chan agent.InteractionResponse, 4)
	var asked, complete bool
	var diagnostic string
	err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: "Plan two alternatives.", Mode: agent.RunModePlan, Preferences: agent.RunPreferences{ApprovalPolicy: "untrusted"}, InteractionResponses: responses}, func(ev agent.Event) {
		switch ev.Type {
		case agent.EventInteractionRequest:
			if strings.Contains(string(ev.Input), `"kind":"plan_review"`) {
				asked = true
				responses <- agent.InteractionResponse{ID: ev.InteractionID, Result: json.RawMessage(`{"decision":"accept","selected_label":"Approach B"}`)}
			} else {
				responses <- agent.InteractionResponse{ID: ev.InteractionID, Result: json.RawMessage(`{"decision":"accept"}`)}
			}
		case agent.EventRunCompleted:
			complete = true
		case agent.EventRunFailed:
			diagnostic = ev.Message
		case agent.EventToolCompleted:
			if ev.IsError {
				t.Errorf("plan tool failed: %s", ev.Output)
			}
		}
	})
	if err != nil || !asked || !selected.Load() || !complete {
		t.Fatalf("err=%v asked=%t selected=%t complete=%t diagnostic=%s", err, asked, selected.Load(), complete, diagnostic)
	}
}

func TestInstalledNativeSideQuestionPreservesMainContext(t *testing.T) {
	var requests atomic.Int32
	home := isolatedNativeCLI(t, func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		step := requests.Add(1)
		if step == 3 && strings.Contains(string(raw), "SIDE_ONLY") {
			t.Error("side conversation leaked into the main context")
		}
		streamNative(w, map[string]any{"content": fmt.Sprintf("ANSWER_%d", step)}, "stop")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session := ""
	for i, prompt := range []string{"Start the main conversation.", "/btw SIDE_ONLY", "Continue the main conversation."} {
		var diagnostic string
		var complete, sideAnswer bool
		err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, ResumeID: session, Prompt: prompt}, func(ev agent.Event) {
			switch ev.Type {
			case agent.EventSessionUpdated:
				session = ev.SessionID
			case agent.EventRunCompleted:
				complete = true
			case agent.EventRunFailed:
				diagnostic = ev.Message
			case agent.EventCollaboration:
				if ev.Status == "completed" && strings.Contains(string(ev.Data), "ANSWER_2") {
					sideAnswer = true
				}
			case agent.EventAssistantTextDelta:
				if i == 1 {
					t.Error("side question emitted text into the main transcript")
				}
			}
		})
		if err != nil || !complete || (i == 1 && !sideAnswer) {
			t.Fatalf("run=%d err=%v complete=%t side=%t diagnostic=%s", i, err, complete, sideAnswer, diagnostic)
		}
	}
	if requests.Load() != 3 {
		t.Fatalf("requests=%d", requests.Load())
	}
}

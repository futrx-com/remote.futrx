package kimi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type fixtureWriter func([]byte) error

func (w fixtureWriter) Write(b []byte) (int, error) {
	if err := w(b); err != nil {
		return 0, err
	}
	return len(b), nil
}
func (w fixtureWriter) Close() error { return nil }
func fixtureTransport(t *testing.T, handler func(map[string]any) (any, int)) *serverTransport {
	t.Helper()
	p := &serverTransport{frames: make(chan bridgeFrame, 64)}
	p.stdin = fixtureWriter(func(b []byte) error {
		var req map[string]any
		if err := json.Unmarshal(b, &req); err != nil {
			return err
		}
		data, code := handler(req)
		raw, _ := json.Marshal(data)
		p.frames <- bridgeFrame{Type: "response", ID: int(req["id"].(float64)), Code: code, Msg: "fixture", Data: raw}
		return nil
	})
	return p
}
func TestNativeResumeRecoveryOnlyAtSessionLookup(t *testing.T) {
	for _, fork := range []bool{false, true} {
		t.Run(fmt.Sprint(fork), func(t *testing.T) {
			calls := 0
			r := newServerRun(agent.RunRequest{ResumeID: "missing", Fork: fork, Cwd: "/workspace"}, func(agent.Event) {})
			p := fixtureTransport(t, func(req map[string]any) (any, int) { calls++; return nil, 40401 })
			err := r.execute(context.Background(), p)
			if !errors.Is(err, agent.ErrSessionNotFound) || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}
func TestNativeRunWaitsForBackgroundTasksAndAppliesPreferences(t *testing.T) {
	for policy, native := range map[string]string{"never": "auto", "on-request": "yolo", "untrusted": "manual"} {
		t.Run(policy, func(t *testing.T) {
			var events []agent.Event
			r := newServerRun(agent.RunRequest{Cwd: "/workspace", Model: "provider/exact[1m]", Mode: agent.RunModePlan, Preferences: agent.RunPreferences{ApprovalPolicy: policy, ReasoningEffort: "high"}}, func(e agent.Event) { events = append(events, e) })
			taskChecks := 0
			promptCalls := 0
			var config map[string]any
			var p *serverTransport
			p = fixtureTransport(t, func(req map[string]any) (any, int) {
				path, _ := req["path"].(string)
				switch path {
				case "/api/v1/sessions":
					return map[string]any{"id": "s"}, 0
				case "/api/v1/sessions/s/snapshot":
					return map[string]any{"epoch": "e", "as_of_seq": 0}, 0
				case "/api/v1/sessions/s/profile":
					config = req["body"].(map[string]any)["agent_config"].(map[string]any)
					return nil, 0
				case "/api/v1/sessions/s/prompts":
					if req["method"] == "POST" {
						promptCalls++
						raw := json.RawMessage(`{"type":"turn.ended","seq":1,"epoch":"e","session_id":"s","payload":{"type":"turn.ended","agentId":"main","reason":"completed"}}`)
						p.frames <- bridgeFrame{Type: "event", Event: raw}
					}
				case "/api/v1/sessions/s/tasks?status=running":
					taskChecks++
					if taskChecks < 3 {
						return map[string]any{"items": []any{map[string]any{"id": "child", "status": "running"}}}, 0
					}
				}
				return map[string]any{}, 0
			})
			p.onEvent = r.onEvent
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := r.execute(ctx, p); err != nil {
				t.Fatal(err)
			}
			if taskChecks < 4 || promptCalls != 1 {
				t.Fatalf("checks=%d prompts=%d", taskChecks, promptCalls)
			}
			if config["permission_mode"] != native || config["model"] != "provider/exact[1m]" || config["thinking"] != "high" || config["plan_mode"] != true {
				t.Fatalf("config=%v", config)
			}
		})
	}
}
func TestNativeAnswersPreserveQuestionIDsMultipleChoicesAndOther(t *testing.T) {
	r := newServerRun(agent.RunRequest{}, func(agent.Event) {})
	r.session = "s"
	emitNative(t, r, 1, false, `{"type":"event.question.requested","question_id":"request-1","questions":[{"id":"q","multi_select":true,"allow_other":true,"options":[{"id":"a","label":"Same label as arbitrary text"},{"id":"b","label":"Second"}]}]}`)
	calls := 0
	var body map[string]any
	p := fixtureTransport(t, func(req map[string]any) (any, int) {
		calls++
		if req["path"] != "/api/v1/sessions/s/questions/request-1" {
			t.Error(req["path"])
		}
		body = req["body"].(map[string]any)
		return nil, 0
	})
	err := r.answer(context.Background(), p, agent.InteractionResponse{ID: "question:request-1", Result: json.RawMessage(`{"answers":{"q":{"answers":["a","b","custom text"]}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), `"kind":"multi_with_other"`) || !strings.Contains(string(raw), `"option_ids":["a","b"]`) {
		t.Fatalf("body=%s", raw)
	}
	_ = r.answer(context.Background(), p, agent.InteractionResponse{ID: "question:request-1", Result: json.RawMessage(`{}`)})
	if calls != 1 {
		t.Fatal("late answer was submitted twice")
	}
}
func TestNativeDismissQuestionAcceptsDismissedResponse(t *testing.T) {
	r := newServerRun(agent.RunRequest{}, func(agent.Event) {})
	r.session = "s"
	r.interactions.pending["question:q"] = pendingInteraction{kind: "question", id: "q"}
	p := fixtureTransport(t, func(req map[string]any) (any, int) {
		if req["path"] != "/api/v1/sessions/s/questions/q:dismiss" {
			t.Error(req["path"])
		}
		return nil, 40909
	})
	if err := r.answer(context.Background(), p, agent.InteractionResponse{ID: "question:q", Result: json.RawMessage(`{"dismiss":true}`)}); err != nil {
		t.Fatal(err)
	}
}
func TestNativeCronKeepsRunOpenUntilDeletedOrOneShotFires(t *testing.T) {
	r := newServerRun(agent.RunRequest{}, func(agent.Event) {})
	r.session = "s"
	r.cron.toolResult(&childTool{Name: "CronCreate", Output: "id: once\nrecurring: false"})
	p := fixtureTransport(t, func(req map[string]any) (any, int) { return map[string]any{}, 0 })
	if idle, err := r.idle(context.Background(), p); err != nil || idle {
		t.Fatalf("idle=%t err=%v", idle, err)
	}
	emitNative(t, r, 1, false, `{"type":"cron.fired","origin":{"jobId":"once"}}`)
	if len(r.cron.jobs) != 0 {
		t.Fatal("one-shot retained after firing")
	}
	r.cron.toolResult(&childTool{Name: "CronCreate", Output: "id: recurring\nrecurring: true"})
	emitNative(t, r, 2, false, `{"type":"cron.fired","origin":{"jobId":"recurring"}}`)
	if len(r.cron.jobs) != 1 {
		t.Fatal("recurring job removed on first firing")
	}
	r.cron.toolResult(&childTool{Name: "CronDelete", Input: json.RawMessage(`{"id":"recurring"}`)})
	if idle, err := r.idle(context.Background(), p); err != nil || !idle {
		t.Fatalf("idle=%t err=%v", idle, err)
	}
}

func TestNativeCommandsUseCurrentUserInputAndPreserveEnrichment(t *testing.T) {
	current := "/agent coder\nInspect this change."
	r := newServerRun(agent.RunRequest{UserPrompt: current, Prompt: "Read the selected skill.\n\n" + current}, func(agent.Event) {})
	r.session = "s"
	p := fixtureTransport(t, func(req map[string]any) (any, int) {
		body := req["body"].(map[string]any)
		if body["profile"] != "coder" {
			t.Fatal("current command not recognized")
		}
		content := body["content"].([]any)[0].(map[string]any)["text"]
		if content != "Read the selected skill.\n\nInspect this change." {
			t.Fatalf("content=%s", content)
		}
		return nil, 0
	})
	if _, err := r.submit(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	r.req = agent.RunRequest{UserPrompt: "Continue the review.", Prompt: "/undo\nContinue the review."}
	p = fixtureTransport(t, func(req map[string]any) (any, int) {
		if req["path"] != "/api/v1/sessions/s/prompts" {
			t.Fatal("context executed a command")
		}
		return nil, 0
	})
	if _, err := r.submit(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func TestNativePlanApprovalCarriesScopeSelectionAndFeedback(t *testing.T) {
	r := newServerRun(agent.RunRequest{}, func(agent.Event) {})
	r.session = "s"
	r.interactions.pending["approval:a"] = pendingInteraction{kind: "approval", id: "a"}
	p := fixtureTransport(t, func(req map[string]any) (any, int) {
		body := req["body"].(map[string]any)
		if body["decision"] != "approved" || body["scope"] != "session" || body["selected_label"] != "Option B" || body["feedback"] != "Keep the API compatible" {
			t.Fatalf("body=%v", body)
		}
		return nil, 0
	})
	if err := r.answer(context.Background(), p, agent.InteractionResponse{ID: "approval:a", Result: json.RawMessage(`{"decision":"acceptForSession","selected_label":"Option B","feedback":"Keep the API compatible"}`)}); err != nil {
		t.Fatal(err)
	}
}

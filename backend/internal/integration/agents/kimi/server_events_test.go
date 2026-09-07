package kimi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func emitNative(t *testing.T, r *serverRun, seq int, volatile bool, payload string) {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"type": data["type"], "session_id": r.session, "seq": seq, "epoch": "e", "volatile": volatile, "payload": data})
	if err := r.onEvent(raw); err != nil {
		t.Fatal(err)
	}
}

func TestNestedTaskCancellationClearsRunningChildWithoutExposingParentTaskAPI(t *testing.T) {
	r := newServerRun(agent.RunRequest{}, func(agent.Event) {})
	r.session = "s"
	emitNative(t, r, 1, false, `{"type":"subagent.spawned","subagentId":"nested","parentAgentId":"parent"}`)
	emitNative(t, r, 2, false, `{"type":"task.started","agentId":"parent","info":{"taskId":"nested-task","agentId":"nested","status":"running"}}`)
	c := r.activity.children["nested"]
	if c.status != "running" || r.activity.taskControlID(r.session, c) != "" {
		t.Fatalf("nested task exposed through main API: %+v", c)
	}
	emitNative(t, r, 3, false, `{"type":"task.terminated","agentId":"parent","info":{"taskId":"nested-task","agentId":"nested","status":"killed"}}`)
	if c.status != "cancelled" {
		t.Fatalf("nested child retained after task cancellation: %+v", c)
	}
}
func TestNativeEventsSeparateNestedAgentsAndCountStepUsageOnce(t *testing.T) {
	var events []agent.Event
	r := newServerRun(agent.RunRequest{}, func(e agent.Event) { events = append(events, e) })
	r.session = "session"
	emitNative(t, r, 1, false, `{"type":"turn.started","agentId":"main","turnId":1}`)
	emitNative(t, r, 2, false, `{"type":"subagent.spawned","subagentId":"a","subagentName":"coder","parentAgentId":"main","parentToolCallId":"parent","runInBackground":true}`)
	emitNative(t, r, 3, false, `{"type":"subagent.spawned","subagentId":"b","subagentName":"explore","parentAgentId":"a","parentToolCallId":"nested"}`)
	emitNative(t, r, 3, true, `{"type":"assistant.delta","agentId":"b","turnId":1,"delta":"child only"}`)
	emitNative(t, r, 3, true, `{"type":"thinking.delta","agentId":"b","turnId":1,"delta":"child reasoning"}`)
	emitNative(t, r, 4, false, `{"type":"tool.call.started","agentId":"b","turnId":1,"toolCallId":"call","name":"Read","args":{"path":"file"}}`)
	emitNative(t, r, 5, false, `{"type":"tool.result","agentId":"b","turnId":1,"toolCallId":"call","output":"missing","isError":true}`)
	step := `{"type":"turn.step.completed","agentId":"b","turnId":1,"step":1,"usage":{"inputOther":10,"output":2,"inputCacheRead":3,"inputCacheCreation":4}}`
	emitNative(t, r, 6, false, step)
	emitNative(t, r, 6, false, step)
	emitNative(t, r, 7, false, step)
	emitNative(t, r, 8, false, `{"type":"subagent.completed","subagentId":"b","resultSummary":"child report","usage":{"inputOther":10,"output":2,"inputCacheRead":3,"inputCacheCreation":4}}`)
	if r.usage.totals.TotalTokens() != 19 {
		t.Fatalf("double-counted usage: %+v", r.usage.totals)
	}
	for _, e := range events {
		if e.Type == agent.EventAssistantTextDelta || e.Type == agent.EventReasoningDelta {
			t.Fatal("child text leaked into main transcript")
		}
	}
	c := r.activity.children["b"]
	if c.parent != "a" || c.status != "completed" || c.thinking != "child reasoning" || len(c.tools) != 1 || !c.tools[0].IsError {
		t.Fatalf("child=%+v", c)
	}
}
func TestNativeFailureAndPrivateInteractionResolution(t *testing.T) {
	var events []agent.Event
	r := newServerRun(agent.RunRequest{}, func(e agent.Event) { events = append(events, e) })
	r.session = "s"
	emitNative(t, r, 1, false, `{"type":"event.question.requested","question_id":"q1","questions":[]}`)
	emitNative(t, r, 2, false, `{"type":"event.question.answered","question_id":"q1","answers":{"private":"SECRET_ANSWER"}}`)
	emitNative(t, r, 3, false, `{"type":"turn.ended","agentId":"main","reason":"failed","error":{"code":"provider.api_error","message":"307 status code (no body)"}}`)
	if !strings.Contains(r.failure, "307") || !r.mainEnded {
		t.Fatalf("failure=%q", r.failure)
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "SECRET_ANSWER") {
		t.Fatal("private response persisted")
	}
	if len(r.interactions.pending) != 0 {
		t.Fatal("resolved request remains pending")
	}
}

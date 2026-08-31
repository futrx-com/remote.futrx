package opencode

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// Fixtures are real `opencode run --format json` lines captured from
// opencode-ai v1.15.13 against a tool-using turn.
const (
	fixtureStepStart  = `{"type":"step_start","timestamp":1788169427440,"sessionID":"ses_abc","part":{"id":"prt_1","messageID":"msg_1","sessionID":"ses_abc","type":"step-start"}}`
	fixtureText       = `{"type":"text","timestamp":1788169431703,"sessionID":"ses_abc","part":{"id":"prt_2","messageID":"msg_2","sessionID":"ses_abc","type":"text","text":"The command ran successfully."}}`
	fixtureToolUse    = `{"type":"tool_use","timestamp":1788169428348,"sessionID":"ses_abc","part":{"type":"tool","tool":"bash","callID":"call_1","state":{"status":"completed","input":{"command":"echo probe123"},"output":"probe123\n"}}}`
	fixtureToolStart  = `{"type":"tool_use","timestamp":1788169428348,"sessionID":"ses_abc","part":{"type":"tool","tool":"bash","callID":"call_2","state":{"status":"pending","input":{"command":"ls"}}}}`
	fixtureToolFinish = `{"type":"step_finish","timestamp":1788169428348,"sessionID":"ses_abc","part":{"id":"prt_3","reason":"tool-calls","messageID":"msg_1","sessionID":"ses_abc","type":"step-finish","tokens":{"total":60404,"input":3019,"output":29,"reasoning":12,"cache":{"write":0,"read":57344}},"cost":0}}`
	fixtureStopFinish = `{"type":"step_finish","timestamp":1788169428999,"sessionID":"ses_abc","part":{"id":"prt_4","reason":"stop","messageID":"msg_2","sessionID":"ses_abc","type":"step-finish","tokens":{"total":10,"input":5,"output":4,"reasoning":1,"cache":{"write":0,"read":0}},"cost":0.25}}`
)

func parseLines(t *testing.T, parser *Parser, lines ...string) []agent.Event {
	t.Helper()
	var events []agent.Event
	for _, line := range lines {
		parsed, err := parser.ParseLine([]byte(line))
		if err != nil {
			t.Fatalf("ParseLine(%s): %v", line, err)
		}
		events = append(events, parsed...)
	}
	return events
}

func TestParserEmitsSessionUpdateOncePerSession(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "conv-1"})
	events := parseLines(t, parser, fixtureStepStart, fixtureText, fixtureText)

	var sessions []agent.Event
	for _, event := range events {
		if event.Type == agent.EventSessionUpdated {
			sessions = append(sessions, event)
		}
	}
	if len(sessions) != 1 || sessions[0].SessionID != "ses_abc" {
		t.Fatalf("session events = %#v, want one ses_abc update", sessions)
	}
}

func TestParserMapsTextAndToolEvents(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "conv-1"})
	events := parseLines(t, parser, fixtureStepStart, fixtureToolUse, fixtureText)

	var toolCompleted, textDelta bool
	for _, event := range events {
		switch {
		case event.Type == agent.EventToolCompleted:
			toolCompleted = true
			if event.ToolName != "bash" || event.ItemID != "call_1" || event.Output != "probe123\n" {
				t.Fatalf("tool completed payload = %#v", event)
			}
			var input map[string]string
			if err := json.Unmarshal(event.Input, &input); err != nil || input["command"] != "echo probe123" {
				t.Fatalf("tool input = %s", event.Input)
			}
		case event.Type == agent.EventAssistantTextDelta:
			textDelta = true
			if event.ItemKind != agent.ItemMessage || event.Text != "The command ran successfully." {
				t.Fatalf("text delta payload = %#v", event)
			}
		}
	}
	if !toolCompleted || !textDelta {
		t.Fatalf("missing events: toolCompleted=%v textDelta=%v (%#v)", toolCompleted, textDelta, events)
	}
}

func TestParserEmitsToolStartedForPendingState(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "conv-1"})
	events := parseLines(t, parser, fixtureStepStart, fixtureToolStart)

	started := slices.ContainsFunc(events, func(event agent.Event) bool {
		return event.Type == agent.EventToolStarted && event.ItemID == "call_2"
	})
	if !started {
		t.Fatalf("pending tool_use did not emit tool.started: %#v", events)
	}
}

func TestParserIgnoresMidStreamFinishAndCompletesOnStop(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "conv-1", Model: "opencode/test-model"})
	events := parseLines(t, parser, fixtureStepStart, fixtureToolFinish, fixtureText)

	if slices.ContainsFunc(events, func(event agent.Event) bool {
		return event.Type == agent.EventRunCompleted
	}) {
		t.Fatalf("tool-calls finish must not complete the run: %#v", events)
	}

	events = parseLines(t, parser, fixtureStopFinish)
	index := slices.IndexFunc(events, func(event agent.Event) bool {
		return event.Type == agent.EventRunCompleted
	})
	if index < 0 {
		t.Fatalf("stop finish did not complete the run: %#v", events)
	}
	var usage agent.Usage
	if err := json.Unmarshal(events[index].Usage, &usage); err != nil {
		t.Fatal(err)
	}
	if usage.InputTokens != 5 || usage.OutputTokens != 4 || usage.ReasoningTokens != 1 {
		t.Fatalf("usage = %#v", usage)
	}
	if usage.CostUSD == nil || *usage.CostUSD != 0.25 {
		t.Fatalf("cost = %v", usage.CostUSD)
	}
	if usage.Model != "opencode/test-model" {
		t.Fatalf("model attribution = %q", usage.Model)
	}
	if !parser.Completed() {
		t.Fatal("parser should be completed after a stop finish")
	}
}

func TestParserCarriesUsageWithoutFinalFinish(t *testing.T) {
	parser := NewParser(agent.RunRequest{ConversationID: "conv-1", Model: "opencode/test-model"})
	parseLines(t, parser, fixtureStepStart, fixtureToolFinish, fixtureText)

	if parser.Completed() {
		t.Fatal("stream without stop finish must not report completed")
	}
	usage := parser.RunUsage()
	if usage.OutputTokens != 29 || usage.CacheReadTokens != 57344 {
		t.Fatalf("accumulated usage = %#v", usage)
	}
}

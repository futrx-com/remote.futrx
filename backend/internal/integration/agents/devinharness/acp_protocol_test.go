package devinharness

import (
	"encoding/json"
	"testing"
)

func TestACPEnvelopeRoundTrip(t *testing.T) {
	original := acpEnvelope{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":2}`),
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded acpEnvelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.JSONRPC != "2.0" || decoded.Method != "initialize" {
		t.Fatalf("decoded = %#v", decoded)
	}
	if string(decoded.ID) != `1` {
		t.Fatalf("id = %q", decoded.ID)
	}
	if string(decoded.Params) != `{"protocolVersion":2}` {
		t.Fatalf("params = %q", decoded.Params)
	}
}

func TestACPEnvelopeResponseRoundTrip(t *testing.T) {
	original := acpEnvelope{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Result:  json.RawMessage(`{"protocolVersion":1}`),
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded acpEnvelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Method != "" {
		t.Fatalf("response should have no method, got %q", decoded.Method)
	}
	if string(decoded.Result) != `{"protocolVersion":1}` {
		t.Fatalf("result = %q", decoded.Result)
	}
}

func TestACPEnvelopeErrorRoundTrip(t *testing.T) {
	original := acpEnvelope{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Error: &acpError{
			Code:    -32602,
			Message: "Invalid params: missing field 'mcpServers'",
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded acpEnvelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Error == nil || decoded.Error.Code != -32602 || decoded.Error.Message != "Invalid params: missing field 'mcpServers'" {
		t.Fatalf("error = %#v", decoded.Error)
	}
}

func TestACPEnvelopeNotificationRoundTrip(t *testing.T) {
	original := acpEnvelope{
		JSONRPC: "2.0",
		Method:  "session/cancel",
		Params:  json.RawMessage(`{"sessionId":"s-1"}`),
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded acpEnvelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.ID) != 0 {
		t.Fatalf("notification should have no id, got %q", decoded.ID)
	}
	if decoded.Method != "session/cancel" {
		t.Fatalf("method = %q", decoded.Method)
	}
}

func TestACPInitializeParamsRoundTrip(t *testing.T) {
	params := acpInitializeParams{
		ProtocolVersion: 2,
		ClientCapabilities: acpClientCapabilities{
			Elicitation: &acpElicitationCapability{},
		},
		ClientInfo: acpClientInfo{
			Name:    "remote.futrx",
			Version: "0.1.0",
		},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var decoded acpInitializeParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ProtocolVersion != 2 {
		t.Fatalf("protocolVersion = %d", decoded.ProtocolVersion)
	}
	if decoded.ClientInfo.Name != "remote.futrx" || decoded.ClientInfo.Version != "0.1.0" {
		t.Fatalf("clientInfo = %#v", decoded.ClientInfo)
	}
	if decoded.ClientCapabilities.Elicitation == nil {
		t.Fatalf("capabilities = %#v", decoded.ClientCapabilities)
	}
	if decoded.ClientCapabilities.FS != nil {
		t.Fatalf("fs capability should be nil, got %#v", decoded.ClientCapabilities.FS)
	}
}

func TestACPInitializeResultAcceptsProtocolVersion1(t *testing.T) {
	raw := json.RawMessage(`{
		"protocolVersion": 1,
		"agentCapabilities": {
			"loadSession": true,
			"promptCapabilities": {"image": true, "audio": false, "embeddedContext": true}
		},
		"authMethods": [{"id":"devin-browser","name":"Log in with browser"}],
		"agentInfo": {"name":"affogato","title":"Devin Agent","version":"0.0.0-dev"}
	}`)
	var result acpInitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.ProtocolVersion != 1 {
		t.Fatalf("protocolVersion = %d, want 1", result.ProtocolVersion)
	}
	if len(result.AuthMethods) != 1 || result.AuthMethods[0].ID != "devin-browser" {
		t.Fatalf("authMethods = %#v", result.AuthMethods)
	}
	if !result.AgentCapabilities.LoadSession {
		t.Fatalf("agentCapabilities = %#v", result.AgentCapabilities)
	}
}

func TestACPSessionNewParamsRoundTrip(t *testing.T) {
	params := acpSessionNewParams{
		Cwd:        "/workspace",
		McpServers: []json.RawMessage{},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var decoded acpSessionNewParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Cwd != "/workspace" {
		t.Fatalf("cwd = %q", decoded.Cwd)
	}
	if len(decoded.McpServers) != 0 {
		t.Fatalf("mcpServers = %#v", decoded.McpServers)
	}
}

func TestACPSessionPromptParamsRoundTrip(t *testing.T) {
	params := acpSessionPromptParams{
		SessionID: "s-1",
		Prompt: []acpContentBlock{
			{Type: "text", Text: "hello world"},
		},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var decoded acpSessionPromptParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SessionID != "s-1" {
		t.Fatalf("sessionId = %q", decoded.SessionID)
	}
	if len(decoded.Prompt) != 1 || decoded.Prompt[0].Type != "text" || decoded.Prompt[0].Text != "hello world" {
		t.Fatalf("prompt = %#v", decoded.Prompt)
	}
}

func TestACPSessionPromptResultRoundTrip(t *testing.T) {
	raw := json.RawMessage(`{"stopReason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)
	var result acpSessionPromptResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopReasonEndTurn {
		t.Fatalf("stopReason = %q, want end_turn", result.StopReason)
	}
	if len(result.Usage) == 0 {
		t.Fatal("usage should be present")
	}
}

func TestACPStopReasonConstants(t *testing.T) {
	tests := []struct {
		value    acpStopReason
		expected string
	}{
		{StopReasonEndTurn, "end_turn"},
		{StopReasonMaxTokens, "max_tokens"},
		{StopReasonMaxTurnRequests, "max_turn_requests"},
		{StopReasonRefusal, "refusal"},
		{StopReasonCancelled, "cancelled"},
	}
	for _, test := range tests {
		if string(test.value) != test.expected {
			t.Fatalf("stop reason = %q, want %q", test.value, test.expected)
		}
	}
}

func TestACPContentBlockTextRoundTrip(t *testing.T) {
	block := acpContentBlock{Type: "text", Text: "hello"}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	var decoded acpContentBlock
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Type != "text" || decoded.Text != "hello" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestACPRequestPermissionParamsRoundTrip(t *testing.T) {
	raw := json.RawMessage(`{
		"sessionId":"s-1",
		"toolCall":{"toolCallId":"tc-1","title":"Bash","status":"pending"},
		"options":[{"optionId":"allow_once","name":"Allow","kind":"allow_once"},{"optionId":"reject_once","name":"Deny","kind":"reject_once"}]
	}`)
	var params acpRequestPermissionParams
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatal(err)
	}
	if params.SessionID != "s-1" {
		t.Fatalf("sessionId = %q", params.SessionID)
	}
	if len(params.Options) != 2 {
		t.Fatalf("options = %#v", params.Options)
	}
	if params.Options[0].OptionID != "allow_once" || params.Options[1].OptionID != "reject_once" {
		t.Fatalf("option ids = %#v", params.Options)
	}
}

func TestACPPermissionResultRoundTrip(t *testing.T) {
	result := acpPermissionResult{
		Outcome: acpPermissionOutcome{Outcome: "selected", OptionID: "allow_once"},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded acpPermissionResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Outcome.Outcome != "selected" || decoded.Outcome.OptionID != "allow_once" {
		t.Fatalf("outcome = %q, optionId = %q", decoded.Outcome.Outcome, decoded.Outcome.OptionID)
	}
}

func TestTranslatePermissionResultGrantTurn(t *testing.T) {
	options := []acpPermissionOption{
		{OptionID: "allow_once", Name: "Allow", Kind: "allow_once"},
		{OptionID: "allow_session", Name: "Yes, allow (this session)", Kind: "allow_always"},
		{OptionID: "reject_once", Name: "Reject", Kind: "reject_once"},
	}
	uiResult := json.RawMessage(`{"permissions":{"tool":true},"scope":"turn"}`)
	translated, err := translatePermissionResult(uiResult, options)
	if err != nil {
		t.Fatal(err)
	}
	var result acpPermissionResult
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Outcome != "selected" || result.Outcome.OptionID != "allow_once" {
		t.Fatalf("outcome = %q, optionId = %q, want selected/allow_once", result.Outcome.Outcome, result.Outcome.OptionID)
	}
}

func TestTranslatePermissionResultGrantSession(t *testing.T) {
	options := []acpPermissionOption{
		{OptionID: "allow_once", Name: "Allow", Kind: "allow_once"},
		{OptionID: "allow_session", Name: "Yes, allow (this session)", Kind: "allow_always"},
		{OptionID: "reject_once", Name: "Reject", Kind: "reject_once"},
	}
	uiResult := json.RawMessage(`{"permissions":{"tool":true},"scope":"session"}`)
	translated, err := translatePermissionResult(uiResult, options)
	if err != nil {
		t.Fatal(err)
	}
	var result acpPermissionResult
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Outcome != "selected" || result.Outcome.OptionID != "allow_session" {
		t.Fatalf("outcome = %q, optionId = %q, want selected/allow_session", result.Outcome.Outcome, result.Outcome.OptionID)
	}
}

func TestTranslatePermissionResultDeny(t *testing.T) {
	options := []acpPermissionOption{
		{OptionID: "allow_once", Name: "Allow", Kind: "allow_once"},
		{OptionID: "reject_once", Name: "Reject", Kind: "reject_once"},
	}
	uiResult := json.RawMessage(`{"permissions":{},"scope":"turn"}`)
	translated, err := translatePermissionResult(uiResult, options)
	if err != nil {
		t.Fatal(err)
	}
	var result acpPermissionResult
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatal(err)
	}
	if result.Outcome.Outcome != "cancelled" {
		t.Fatalf("outcome = %q, want cancelled", result.Outcome.Outcome)
	}
}

func TestACPCreateElicitationParamsRoundTrip(t *testing.T) {
	raw := json.RawMessage(`{
		"elicitationId":"el-1",
		"requestedSchema":{"type":"object","properties":{"name":{"type":"string"}}},
		"toolCallId":"tc-1"
	}`)
	var params acpCreateElicitationParams
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatal(err)
	}
	if params.ElicitationID != "el-1" {
		t.Fatalf("elicitationId = %q", params.ElicitationID)
	}
	if params.ToolCallID != "tc-1" {
		t.Fatalf("toolCallId = %q", params.ToolCallID)
	}
	if len(params.RequestedSchema) == 0 {
		t.Fatal("requestedSchema should be present")
	}
}

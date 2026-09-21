package devinharness

import (
	"encoding/json"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// acpRequestID enumerates the client-to-agent JSON-RPC requests in the order
// they are sent during one run. Numeric IDs keep the wire protocol compact and
// match the pattern used by codexharness.
type acpRequestID int

const (
	acpInitializeRequestID acpRequestID = iota + 1
	acpSessionRequestID
	acpPromptRequestID
)

// clientVersion is the version reported in the ACP initialize clientInfo. It
// identifies Remote as the ACP host without claiming Windsurf identity.
const clientVersion = "0.1.0"

// buildInitialize constructs the ACP initialize request. The client requests
// protocol version 2; the agent may respond with a lower version (confirmed
// v1 by live traffic — the harness accepts whatever the agent returns).
//
// FS capabilities are intentionally NOT declared. Devin runs inside the
// container with direct filesystem access, so it does not need the host to
// read or write files on its behalf. Declaring fs.readTextFile/fs.writeTextFile
// causes Devin to send fs/read_text_file and fs/write_text_file host-tool
// requests, which the harness has no handler for — they would surface as
// generic JSON input prompts in the UI. Devin reads and writes files itself.
func buildInitialize() map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      acpInitializeRequestID,
		"method":  "initialize",
		"params": acpInitializeParams{
			ProtocolVersion: 2,
			ClientCapabilities: acpClientCapabilities{
				Elicitation: &acpElicitationCapability{},
			},
			ClientInfo: acpClientInfo{
				Name:    "remote.futrx",
				Version: clientVersion,
			},
		},
	}
}

// buildSessionNew constructs the session/new request. mcpServers is required
// (can be empty); without it the server returns "missing field 'mcpServers'".
func buildSessionNew(req agent.RunRequest) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      acpSessionRequestID,
		"method":  "session/new",
		"params": acpSessionNewParams{
			Cwd:        strings.TrimSpace(req.Cwd),
			McpServers: []json.RawMessage{},
		},
	}
}

// buildSessionResume constructs the session/resume request for continuing an
// existing conversation.
func buildSessionResume(req agent.RunRequest) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      acpSessionRequestID,
		"method":  "session/resume",
		"params": acpSessionResumeParams{
			SessionID:  req.ResumeID,
			Cwd:        strings.TrimSpace(req.Cwd),
			McpServers: []json.RawMessage{},
		},
	}
}

// buildSessionPrompt constructs the session/prompt request. The prompt is a
// slice of ContentBlocks; for text input it is a single text block.
func buildSessionPrompt(sessionID, prompt string) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      acpPromptRequestID,
		"method":  "session/prompt",
		"params": acpSessionPromptParams{
			SessionID: sessionID,
			Prompt: []acpContentBlock{
				{Type: "text", Text: prompt},
			},
		},
	}
}

// buildSessionCancel constructs the session/cancel notification. It is a
// notification (no id, no response expected). The cancelled turn is confirmed
// by the session/prompt response arriving with stopReason "cancelled".
func buildSessionCancel(sessionID string) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/cancel",
		"params":  acpSessionCancelParams{SessionID: sessionID},
	}
}

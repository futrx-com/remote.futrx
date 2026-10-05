package devinharness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// acpRequestHandler stores pending server-to-client requests (permission
// prompts, elicitations) by JSON-RPC id, emits EventInteractionRequest, and
// routes InteractionResponse back when the run-scoped channel delivers a user
// answer.
type acpRequestHandler struct {
	req     agent.RunRequest
	emit    func(agent.Event)
	write   func(any) error
	pending map[string]acpPendingRequest
}

type acpPendingRequest struct {
	envelope  acpEnvelope
	createdAt time.Time
	// permissionOptions stores the optionId values from session/request_permission
	// so the UI's generic grant/deny response can be mapped back to Devin's
	// expected outcome format.
	permissionOptions []acpPermissionOption
}

func newACPRequestHandler(
	req agent.RunRequest,
	emit func(agent.Event),
	write func(any) error,
) *acpRequestHandler {
	return &acpRequestHandler{
		req:     req,
		emit:    emit,
		write:   write,
		pending: make(map[string]acpPendingRequest),
	}
}

// Handle stores a server-to-client request and emits an EventInteractionRequest
// for the UI. The request ID is preserved verbatim so string and numeric IDs
// remain distinct.
func (handler *acpRequestHandler) Handle(envelope acpEnvelope) error {
	requestID, err := jsonRPCIDKey(envelope.ID)
	if err != nil {
		return err
	}
	if _, exists := handler.pending[requestID]; exists {
		return fmt.Errorf("duplicate ACP request %s", requestID)
	}

	pending := acpPendingRequest{envelope: envelope, createdAt: time.Now()}

	// For session/request_permission, translate Devin's native options into
	// the provider-neutral format the UI's PermissionInteractionForm expects.
	// The UI looks for a "permissions" field to render the grant/deny buttons.
	var uiInput json.RawMessage
	if envelope.Method == "session/request_permission" {
		var params acpRequestPermissionParams
		if err := json.Unmarshal(envelope.Params, &params); err == nil {
			pending.permissionOptions = params.Options
			uiInput = translatePermissionParams(envelope.Params, params)
		} else {
			uiInput = cloneRaw(envelope.Params)
		}
	} else {
		uiInput = cloneRaw(envelope.Params)
	}

	handler.pending[requestID] = pending
	handler.emit(agent.Event{
		T:              time.Now().UnixMilli(),
		Type:           agent.EventInteractionRequest,
		Provider:       handler.req.Provider,
		ConversationID:  handler.req.ConversationID,
		ToolName:       envelope.Method,
		Input:          uiInput,
		InteractionID:  requestID,
		Status:         interactionKind(envelope.Method),
		Native: &agent.NativeEnvelope{
			SchemaVersion: agent.NativeEnvelopeSchemaVersion,
			Method:        envelope.Method,
			RequestID:     requestID,
			Payload:       cloneRaw(envelope.Params),
		},
	})
	return nil
}

// Respond routes a user-supplied InteractionResponse back to the ACP server.
func (handler *acpRequestHandler) Respond(response agent.InteractionResponse) error {
	pending, ok := handler.pending[response.ID]
	if !ok {
		return fmt.Errorf("%w: %s", errors.New("unknown ACP interaction"), response.ID)
	}

	request := pending.envelope
	wire := map[string]any{"jsonrpc": "2.0", "id": request.ID}
	switch {
	case len(response.Error) > 0:
		if !json.Valid(response.Error) {
			return errors.New("invalid JSON-RPC interaction error")
		}
		wire["error"] = json.RawMessage(response.Error)
	default:
		result := response.Result
		if len(result) == 0 {
			result = json.RawMessage("null")
		}
		if !json.Valid(result) {
			return errors.New("invalid JSON-RPC interaction result")
		}
		// For session/request_permission, translate the UI's generic
		// grant_permissions/deny_permissions response into Devin's expected
		// outcome format with the matching optionId.
		if request.Method == "session/request_permission" {
			translated, err := translatePermissionResult(result, pending.permissionOptions)
			if err != nil {
				return err
			}
			result = translated
		}
		wire["result"] = json.RawMessage(result)
	}

	if err := handler.write(wire); err != nil {
		return err
	}
	delete(handler.pending, response.ID)
	handler.emit(handler.resolvedEvent(request, response.ID, interactionResponseStatus(request.Method, response.Result, response.Error)))
	return nil
}

// ResolveAll emits resolved events for all pending requests without sending
// responses to the server. Used when the turn ends.
func (handler *acpRequestHandler) ResolveAll(status string) {
	for requestID, pending := range handler.pending {
		handler.emit(handler.resolvedEvent(pending.envelope, requestID, status))
		delete(handler.pending, requestID)
	}
}

func (handler *acpRequestHandler) resolvedEvent(request acpEnvelope, requestID string, status string) agent.Event {
	return agent.Event{
		T:              time.Now().UnixMilli(),
		Type:           agent.EventInteractionDone,
		Provider:       handler.req.Provider,
		ConversationID: handler.req.ConversationID,
		ToolName:       request.Method,
		InteractionID:  requestID,
		Status:         status,
		Native: &agent.NativeEnvelope{
			SchemaVersion: agent.NativeEnvelopeSchemaVersion,
			Method:        request.Method,
			RequestID:     requestID,
		},
	}
}

func jsonRPCIDKey(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || !json.Valid(raw) {
		return "", errors.New("ACP request has an invalid id")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", err
	}
	key := compact.String()
	if key == "null" || key == "" || (key[0] != '"' && !strings.ContainsAny(key[:1], "-0123456789")) {
		return "", errors.New("ACP request id must be a string or number")
	}
	return key, nil
}

func interactionKind(method string) string {
	switch method {
	case "session/request_permission":
		return "permission"
	case "session/elicitation/create":
		return "elicitation"
	default:
		return "provider_request"
	}
}

func interactionResponseStatus(method string, result, responseError json.RawMessage) string {
	if len(responseError) > 0 {
		return "response_error"
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(result, &value) != nil {
		return "answered"
	}
	if raw, exists := value["outcome"]; exists {
		var outcome string
		if json.Unmarshal(raw, &outcome) == nil {
			switch outcome {
			case "allow", "accepted":
				return "approved"
			case "deny", "denied":
				return "denied"
			}
		}
	}
	if raw, exists := value["action"]; exists {
		var action string
		if json.Unmarshal(raw, &action) == nil {
			switch action {
			case "accept":
				return "accepted"
			case "decline":
				return "denied"
			case "cancel":
				return "cancelled"
			}
		}
	}
	return "answered"
}

// translatePermissionParams converts Devin's session/request_permission
// params into the provider-neutral format the UI's PermissionInteractionForm
// expects. The UI looks for a "permissions" field to render grant/deny buttons.
func translatePermissionParams(raw json.RawMessage, params acpRequestPermissionParams) json.RawMessage {
	// Build a UI-friendly input that includes the original fields plus a
	// "permissions" field so the PermissionInteractionForm renders correctly.
	var base map[string]json.RawMessage
	if json.Unmarshal(raw, &base) != nil {
		base = make(map[string]json.RawMessage)
	}
	// Extract tool call info for display.
	var toolCall map[string]json.RawMessage
	if json.Unmarshal(params.ToolCall, &toolCall) == nil {
		for k, v := range toolCall {
			if k == "_meta" {
				base["reason"] = v
			}
		}
	}
	// Add a non-empty permissions object so the UI form renders.
	base["permissions"] = json.RawMessage(`{"tool":true}`)
	encoded, err := json.Marshal(base)
	if err != nil {
		return raw
	}
	return encoded
}

// translatePermissionResult converts the UI's generic grant_permissions /
// deny_permissions response into Devin's expected ACP v1 format.
//
// The ACP schema uses:
//   struct RequestPermissionResponse { outcome: RequestPermissionOutcome }
//   #[serde(tag = "outcome", rename_all = "snake_case")]
//   enum RequestPermissionOutcome { Cancelled, Selected(SelectedPermissionOutcome) }
//
// So the wire format is:
//   {"outcome": {"outcome": "selected", "optionId": "allow_once"}}
//   {"outcome": {"outcome": "cancelled"}}
func translatePermissionResult(uiResult json.RawMessage, options []acpPermissionOption) (json.RawMessage, error) {
	var uiResponse struct {
		Permissions json.RawMessage `json:"permissions"`
		Scope       string          `json:"scope"`
	}
	if err := json.Unmarshal(uiResult, &uiResponse); err != nil {
		// Not a grant_permissions response — pass through as-is.
		return uiResult, nil
	}

	// If permissions is empty/null, the user denied.
	if len(uiResponse.Permissions) == 0 || string(uiResponse.Permissions) == "null" || string(uiResponse.Permissions) == "{}" {
		return json.Marshal(acpPermissionResult{
			Outcome: acpPermissionOutcome{Outcome: "cancelled"},
		})
	}

	// User granted — map scope to the appropriate option.
	var optionID string
	if uiResponse.Scope == "session" {
		optionID = findOptionID(options, "allow_session")
		if optionID == "" {
			optionID = findOptionID(options, "allow_always")
		}
	} else {
		optionID = findOptionID(options, "allow_once")
		if optionID == "" {
			optionID = findOptionID(options, "allow")
		}
	}
	if optionID == "" && len(options) > 0 {
		optionID = options[0].OptionID
	}
	if optionID == "" {
		optionID = "allow_once"
	}
	return json.Marshal(acpPermissionResult{
		Outcome: acpPermissionOutcome{Outcome: "selected", OptionID: optionID},
	})
}

// findOptionID searches for an option whose optionId contains the given substring.
func findOptionID(options []acpPermissionOption, contains string) string {
	for _, opt := range options {
		if strings.Contains(opt.OptionID, contains) {
			return opt.OptionID
		}
	}
	return ""
}

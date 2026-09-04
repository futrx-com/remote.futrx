package codexharness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type appServerRequestHandler struct {
	req     agent.RunRequest
	emit    func(agent.Event)
	write   func(any) error
	pending map[string]appServerPendingRequest
}

type appServerPendingRequest struct {
	envelope  appServerEnvelope
	createdAt time.Time
}

func newAppServerRequestHandler(
	req agent.RunRequest,
	emit func(agent.Event),
	write func(any) error,
) *appServerRequestHandler {
	return &appServerRequestHandler{
		req:     req,
		emit:    emit,
		write:   write,
		pending: make(map[string]appServerPendingRequest),
	}
}

// Handle keeps provider requests pending until the browser supplies a
// response. App-server request IDs are preserved verbatim so a string ID is
// never accidentally answered as a numeric ID (or vice versa).
func (handler *appServerRequestHandler) Handle(envelope appServerEnvelope) error {
	requestID, err := jsonRPCIDKey(envelope.ID)
	if err != nil {
		return err
	}
	if _, exists := handler.pending[requestID]; exists {
		return fmt.Errorf("duplicate app-server request %s", requestID)
	}

	ids := nativeIDs(envelope.Params)
	handler.pending[requestID] = appServerPendingRequest{envelope: envelope, createdAt: time.Now()}
	handler.emit(agent.Event{
		T:              time.Now().UnixMilli(),
		Type:           agent.EventInteractionRequest,
		Provider:       handler.req.Provider,
		ConversationID: handler.req.ConversationID,
		ItemID:         ids.ItemID,
		ItemKind:       agent.ItemToolCall,
		ToolName:       envelope.Method,
		Input:          cloneRaw(envelope.Params),
		InteractionID:  requestID,
		Status:         interactionKind(envelope.Method),
		Native: &agent.NativeEnvelope{
			SchemaVersion: agent.NativeEnvelopeSchemaVersion,
			Method:        envelope.Method,
			ThreadID:      ids.ThreadID,
			TurnID:        ids.TurnID,
			ItemID:        ids.ItemID,
			RequestID:     requestID,
			Payload:       cloneRaw(envelope.Params),
		},
	})
	return nil
}

func (handler *appServerRequestHandler) Respond(response agent.InteractionResponse) error {
	pending, ok := handler.pending[response.ID]
	if !ok {
		return fmt.Errorf("%w: %s", errors.New("unknown app-server interaction"), response.ID)
	}

	request := pending.envelope
	wire := map[string]any{"id": request.ID}
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
		wire["result"] = json.RawMessage(result)
	}

	if err := handler.write(wire); err != nil {
		return err
	}
	delete(handler.pending, response.ID)
	handler.emit(handler.resolvedEvent(
		request,
		response.ID,
		interactionResponseStatus(request.Method, response.Result, response.Error),
	))
	return nil
}

func (handler *appServerRequestHandler) Resolve(params json.RawMessage) *agent.Event {
	var resolved appServerRequestResolvedParams
	if err := json.Unmarshal(params, &resolved); err != nil {
		return nil
	}
	requestID, err := jsonRPCIDKey(resolved.RequestID)
	if err != nil {
		return nil
	}
	pending, exists := handler.pending[requestID]
	delete(handler.pending, requestID)
	if !exists {
		return nil
	}
	status := "server_cancelled"
	var userInput appServerUserInputRequestParams
	if json.Unmarshal(pending.envelope.Params, &userInput) == nil && userInput.AutoResolutionMs != nil {
		deadline := pending.createdAt.Add(time.Duration(*userInput.AutoResolutionMs) * time.Millisecond)
		if !time.Now().Before(deadline) {
			status = "timed_out"
		}
	}
	event := handler.resolvedEvent(pending.envelope, requestID, status)
	event.Native.Payload = cloneRaw(params)
	if resolved.ThreadID != "" {
		event.Native.ThreadID = resolved.ThreadID
	}
	return &event
}

func (handler *appServerRequestHandler) ResolveAll(status string) {
	for requestID, pending := range handler.pending {
		handler.emit(handler.resolvedEvent(pending.envelope, requestID, status))
		delete(handler.pending, requestID)
	}
}

func (handler *appServerRequestHandler) resolvedEvent(
	request appServerEnvelope,
	requestID string,
	status string,
) agent.Event {
	ids := nativeIDs(request.Params)
	return agent.Event{
		T:              time.Now().UnixMilli(),
		Type:           agent.EventInteractionDone,
		Provider:       handler.req.Provider,
		ConversationID: handler.req.ConversationID,
		ItemID:         ids.ItemID,
		ToolName:       request.Method,
		InteractionID:  requestID,
		Status:         status,
		Native: &agent.NativeEnvelope{
			SchemaVersion: agent.NativeEnvelopeSchemaVersion,
			Method:        "serverRequest/resolved",
			ThreadID:      ids.ThreadID,
			TurnID:        ids.TurnID,
			ItemID:        ids.ItemID,
			RequestID:     requestID,
		},
	}
}

func jsonRPCIDKey(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || !json.Valid(raw) {
		return "", errors.New("app-server request has an invalid id")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", err
	}
	key := compact.String()
	if key == "null" || key == "" || (key[0] != '"' && !strings.Contains("-0123456789", key[:1])) {
		return "", errors.New("app-server request id must be a string or number")
	}
	return key, nil
}

func interactionKind(method string) string {
	switch method {
	case "item/tool/requestUserInput", "tool/requestUserInput":
		return "user_input"
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "execCommandApproval", "applyPatchApproval":
		return "approval"
	case "item/permissions/requestApproval":
		return "permission"
	case "mcpServer/elicitation/request":
		return "elicitation"
	default:
		return "provider_request"
	}
}

func interactionResponseStatus(method string, result, responseError json.RawMessage) string {
	if len(responseError) > 0 {
		var rpcError struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(responseError, &rpcError) == nil && rpcError.Code == -32601 {
			return "unsupported"
		}
		return "response_error"
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(result, &value) != nil {
		return "answered"
	}
	if raw, exists := value["decision"]; exists {
		var decision string
		if json.Unmarshal(raw, &decision) == nil {
			switch decision {
			case "accept", "approved":
				return "approved"
			case "acceptForSession", "approved_for_session":
				return "approved_for_session"
			case "decline", "abort":
				return "denied"
			case "cancel":
				return "cancelled"
			}
		}
		var review map[string]json.RawMessage
		if json.Unmarshal(raw, &review) == nil {
			if _, denied := review["denied"]; denied {
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
	if raw, exists := value["permissions"]; exists {
		var permissions map[string]json.RawMessage
		if json.Unmarshal(raw, &permissions) == nil && len(permissions) == 0 {
			return "denied"
		}
		var scope string
		_ = json.Unmarshal(value["scope"], &scope)
		if scope == "session" {
			return "granted_session"
		}
		return "granted_turn"
	}
	if method == "item/tool/requestUserInput" || method == "tool/requestUserInput" {
		return "answered"
	}
	return "answered"
}

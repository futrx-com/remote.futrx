package codex

import (
	"encoding/json"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type appServerRequestHandler struct {
	req   agent.RunRequest
	emit  func(agent.Event)
	write func(any) error
}

func newAppServerRequestHandler(
	req agent.RunRequest,
	emit func(agent.Event),
	write func(any) error,
) *appServerRequestHandler {
	return &appServerRequestHandler{req: req, emit: emit, write: write}
}

func (handler *appServerRequestHandler) Answer(envelope appServerEnvelope) error {
	result := any(nil)
	switch envelope.Method {
	case "item/tool/requestUserInput", "tool/requestUserInput":
		var params appServerUserInputRequestParams
		if err := json.Unmarshal(envelope.Params, &params); err != nil {
			return err
		}
		input, _ := json.Marshal(map[string]any{"questions": params.Questions})
		handler.emit(agent.Event{
			T:              time.Now().UnixMilli(),
			Type:           agent.EventToolStarted,
			Provider:       agent.ProviderCodex,
			ConversationID: handler.req.ConversationID,
			ItemID:         params.ItemID,
			ItemKind:       agent.ItemToolCall,
			ToolName:       "AskUserQuestion",
			Input:          input,
		})
		answers := make(map[string]any, len(params.Questions))
		for _, question := range params.Questions {
			answers[question.ID] = map[string]any{"answers": []string{}}
		}
		result = map[string]any{"answers": answers}

	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		decision := "accept"
		if handler.req.Mode == agent.RunModePlan {
			decision = "decline"
		}
		result = map[string]string{"decision": decision}

	case "execCommandApproval", "applyPatchApproval":
		if handler.req.Mode == agent.RunModePlan {
			result = map[string]any{"decision": map[string]any{
				"denied": map[string]string{"rejection": "Plan mode does not allow mutations"},
			}}
		} else {
			result = map[string]string{"decision": "approved"}
		}

	case "mcpServer/elicitation/request":
		result = map[string]any{"action": "cancel", "content": nil}

	default:
		return handler.write(map[string]any{
			"id": envelope.ID,
			"error": map[string]any{
				"code":    -32601,
				"message": "Remote does not implement " + envelope.Method,
			},
		})
	}
	return handler.write(map[string]any{"id": envelope.ID, "result": result})
}

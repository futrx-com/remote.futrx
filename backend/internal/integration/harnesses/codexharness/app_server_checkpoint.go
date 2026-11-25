package codexharness

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

// verifyForkCheckpoint checks the native fork before accepting it as the new
// session. Older CLIs can ignore an unknown lastTurnId field, so a successful
// thread/fork response alone does not prove the discarded turns were omitted.
func (run *appServerRun) verifyForkCheckpoint() error {
	if run.threadID == run.req.ResumeID {
		return fmt.Errorf("%w: %s fork reused the source thread", harness.ErrNativeCheckpointUnavailable, run.providerLabel)
	}
	return run.process.write(map[string]any{
		"method": "thread/turns/list",
		"id":     appServerCheckpointRequestID,
		"params": map[string]any{
			"threadId":      run.threadID,
			"limit":         1,
			"sortDirection": "desc",
			"itemsView":     "notLoaded",
		},
	})
}

func (run *appServerRun) handleForkCheckpoint(raw json.RawMessage) error {
	var result struct {
		Data []appServerTurnResult `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("%w: decode %s fork checkpoint: %v", harness.ErrNativeCheckpointUnavailable, run.providerLabel, err)
	}
	if len(result.Data) != 1 || result.Data[0].ID != run.req.NativeCheckpoint || result.Data[0].Status != "completed" {
		return fmt.Errorf("%w: %s fork did not retain the requested completed turn boundary", harness.ErrNativeCheckpointUnavailable, run.providerLabel)
	}
	return run.startThreadTurn()
}

func (run *appServerRun) startThreadTurn() error {
	model := run.eventParser.req.Model
	if run.threadID != run.req.ResumeID {
		run.emit(harness.Event{
			T:              time.Now().UnixMilli(),
			Type:           harness.EventSessionUpdated,
			Provider:       run.req.Provider,
			ConversationID: run.req.ConversationID,
			SessionID:      run.threadID,
			Model:          model,
		})
	}
	return run.process.write(map[string]any{
		"method": "turn/start",
		"id":     appServerTurnRequestID,
		"params": buildAppServerTurnParams(run.req, run.threadID, model),
	})
}

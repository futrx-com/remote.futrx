package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func (r *serverRun) controlTask(ctx context.Context, p *serverTransport, taskID string, response agent.InteractionResponse) error {

	if !r.activity.controlsTask(taskID) {
		return nil
	}
	var action struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(response.Result, &action) != nil || (action.Action != "cancel" && action.Action != "detach") {
		return fmt.Errorf("invalid Kimi task action")
	}
	err := p.api(ctx, "POST", r.path()+"/tasks/"+url.PathEscape(taskID)+":"+action.Action, map[string]any{}, nil)
	if err == nil && action.Action == "detach" {
		r.activity.detachTask(taskID, func(c *childAgent) { r.publishChild(c, nil) })
	}
	if e, ok := err.(*serverError); ok && e.Code == 40904 {
		return nil
	}
	return err
}

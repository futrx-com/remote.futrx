package kimi

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func (r *serverRun) interactionEvent(kind string, raw json.RawMessage) error {
	ev, err := r.interactions.event(kind, raw)
	if ev != nil {
		r.publish(*ev)
	}
	return err
}

func (r *serverRun) answer(ctx context.Context, p *serverTransport, response agent.InteractionResponse) error {
	if taskID, ok := strings.CutPrefix(response.ID, "task:"+r.session+":"); ok {
		return r.controlTask(ctx, p, taskID, response)
	}
	pending, ok := r.interactions.get(response.ID)
	if !ok {
		return nil
	} // Late answers cannot target another request.
	reply, err := pending.reply(response)
	if err != nil {
		return err
	}
	err = p.api(ctx, "POST", r.path()+reply.Path, reply.Body, nil)
	if e, ok := err.(*serverError); ok && (e.Code == 40902 || e.Code == 40909 || e.Code == 40404 || e.Code == 40405) {
		err = nil
	}
	if err != nil {
		return err
	}
	if r.interactions.resolve(response.ID) {
		r.publish(agent.Event{Type: agent.EventInteractionDone, InteractionID: response.ID, ToolName: "kimi/" + pending.kind, Status: "resolved"})
	}
	return nil
}

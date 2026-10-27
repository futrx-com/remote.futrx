package kimi

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

type serverRun struct {
	thinking     string
	cron         cronTracker
	compacting   bool
	req          agent.RunRequest
	emit         func(agent.Event)
	session      string
	mainEnded    bool
	failure      string
	interrupted  bool
	seq          int64
	epoch        string
	activity     agentActivity
	interactions interactionRequests
	usage        runUsage
}

func newServerRun(req agent.RunRequest, emit func(agent.Event)) *serverRun {
	return &serverRun{
		req: req, emit: emit,
		cron: newCronTracker(), activity: newAgentActivity(),
		interactions: newInteractionRequests(), usage: newRunUsage(req.Model),
	}
}
func (r *serverRun) publish(ev agent.Event) {
	ev.T = time.Now().UnixMilli()
	ev.Provider = agent.ProviderKimi
	ev.ConversationID = r.req.ConversationID
	ev.SessionID = r.session
	r.emit(ev)
}
func (r *serverRun) publishChild(c *childAgent, native *agent.NativeEnvelope) {
	if ev := r.activity.collaboration(r.session, c, native); ev != nil {
		r.publish(*ev)
	}
}

func (r *serverRun) execute(ctx context.Context, p *serverTransport) error {
	if err := r.openSession(ctx, p); err != nil {
		return err
	}
	if err := r.applyPreferences(ctx, p); err != nil {
		return err
	}
	done, err := r.submit(ctx, p)
	if err != nil || done {
		return err
	}

	ticker := time.NewTicker(configconstants.KimiRunIdlePollInterval)
	defer ticker.Stop()
	responses := r.req.InteractionResponses
	// Confirm idle twice so background task completion callbacks can enqueue
	// follow-up turns. The main agent's first turn.ended is not run completion.
	idleCount := 0
	for {
		if r.failure != "" {
			return errors.New(r.failure)
		}
		if r.interrupted {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case frame, ok := <-p.frames:
			if !ok {
				return errors.New("Kimi event stream closed before completion")
			}
			if frame.Type == "fatal" {
				return errors.New(frame.Message)
			}
			if frame.Type != "event" {
				return errors.New("unexpected Kimi bridge response")
			}
			if err := r.onEvent(frame.Event); err != nil {
				return err
			}
			idleCount = 0
		case response, ok := <-responses:
			if !ok {
				responses = nil
				continue
			}
			if err := r.answer(ctx, p, response); err != nil {
				return err
			}
		case <-ticker.C:
			if r.compacting || !r.mainEnded || r.interactions.active() {
				idleCount = 0
				continue
			}
			idle, err := r.idle(ctx, p)
			if err != nil {
				return err
			}
			if r.failure != "" {
				return errors.New(r.failure)
			}
			if r.interrupted {
				return nil
			}
			if !idle || !r.mainEnded || r.interactions.active() {
				idleCount = 0
				continue
			}
			idleCount++
			if idleCount >= 2 {
				return nil
			}
		}
	}
}

func (r *serverRun) idle(ctx context.Context, p *serverTransport) (bool, error) {
	var status struct {
		Busy bool `json:"busy"`
	}
	if err := p.api(ctx, "GET", r.path()+"/status", nil, &status); err != nil {
		return false, err
	}
	if status.Busy {
		return false, nil
	}
	if err := r.cron.save(ctx, p, r.path()); err != nil {
		return false, err
	}
	if r.cron.active() {
		return false, nil
	}
	var tasks nativeTasks
	if err := p.api(ctx, "GET", r.path()+"/tasks?status=running", nil, &tasks); err != nil {
		return false, err
	}
	if len(tasks.Items) > 0 {
		return false, nil
	}
	var prompts struct {
		Items []struct {
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := p.api(ctx, "GET", r.path()+"/prompts", nil, &prompts); err != nil {
		return false, err
	}
	for _, prompt := range prompts.Items {
		if prompt.Status == "queued" || prompt.Status == "running" {
			return false, nil
		}
	}
	var goal *struct {
		Status string `json:"status"`
	}
	if err := p.api(ctx, "GET", r.path()+"/goal", nil, &goal); err != nil {
		return false, err
	}
	if goal != nil && goal.Status == "active" {
		return false, nil
	}
	return !r.activity.running(), nil
}

func (r *serverRun) abort(p *serverTransport) {
	if r.session == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), configconstants.KimiRunAbortTimeout)
	defer cancel()
	_ = r.cron.save(ctx, p, r.path())
	// Server shutdown disposes all agents too. First persist cancellations.
	_ = p.api(ctx, "POST", r.path()+":abort", map[string]any{}, nil)
	var tasks nativeTasks
	if p.api(ctx, "GET", r.path()+"/tasks?status=running", nil, &tasks) == nil {
		for _, task := range tasks.Items {
			_ = p.api(ctx, "POST", r.path()+"/tasks/"+url.PathEscape(task.ID)+":cancel", map[string]any{}, nil)
		}
	}
}

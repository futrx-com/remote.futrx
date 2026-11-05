package kimi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func (r *serverRun) path() string { return "/api/v1/sessions/" + url.PathEscape(r.session) }

type nativeSession struct {
	ID       string `json:"id"`
	Metadata struct {
		Cwd      string          `json:"cwd"`
		CronJobs map[string]bool `json:"remote_kimi_cron_jobs"`
	} `json:"metadata"`
}
type nativeSnapshot struct {
	Seq   int64  `json:"as_of_seq"`
	Epoch string `json:"epoch"`
}
type nativeTasks struct {
	Items []struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		AgentID string `json:"agent_id"`
	} `json:"items"`
}

// openSession establishes native identity before preferences or commands execute.
func (r *serverRun) openSession(ctx context.Context, p *serverTransport) error {
	cwd := r.req.Cwd
	if r.req.ProjectID != "" {
		if cwd == "" {
			cwd = agent.ProjectWorkspacePath
		}
	} else if cwd == "" {
		cwd = os.Getenv("HOME")
		if cwd == "" {
			cwd = "/root"
		}
	}
	if err := r.prepareBrowser(ctx, p); err != nil {
		return err
	}
	var session nativeSession
	if r.req.ResumeID != "" {
		r.session = r.req.ResumeID
		if err := p.api(ctx, "GET", r.path(), nil, &session); err != nil {
			var apiErr *serverError
			if errors.As(err, &apiErr) && apiErr.Code == 40401 {
				return agent.ErrSessionNotFound
			}
			return err
		}
		if session.Metadata.Cwd != "" && filepath.Clean(session.Metadata.Cwd) != filepath.Clean(cwd) {
			return fmt.Errorf("Kimi session belongs to %s, not %s", session.Metadata.Cwd, cwd)
		}
		if r.req.Fork {
			if err := p.api(ctx, "POST", r.path()+":fork", map[string]any{}, &session); err != nil {
				return err
			}
		}
	} else {
		if err := p.api(ctx, "POST", "/api/v1/sessions", map[string]any{"metadata": map[string]any{"cwd": cwd}}, &session); err != nil {
			return err
		}
	}
	if session.ID == "" {
		return errors.New("Kimi did not return a session ID")
	}
	r.session = session.ID
	r.cron.restore(session.Metadata.CronJobs)
	r.publish(agent.Event{Type: agent.EventSessionUpdated})
	var snapshot nativeSnapshot
	if err := p.api(ctx, "GET", r.path()+"/snapshot", nil, &snapshot); err != nil {
		return err
	}
	r.seq = snapshot.Seq
	r.epoch = snapshot.Epoch
	if err := p.request(ctx, bridgeRequest{Type: "subscribe", Payload: &nativeSubscription{Sessions: []string{r.session}, Cursors: map[string]nativeCursor{r.session: {Seq: r.seq, Epoch: r.epoch}}}}, nil); err != nil {
		return err
	}
	var status struct {
		Model string `json:"model"`
		Busy  bool   `json:"busy"`
	}
	if err := p.api(ctx, "GET", r.path()+"/status", nil, &status); err != nil {
		return err
	}
	if status.Busy {
		return errors.New("Kimi session already has an active turn")
	}
	return nil
}

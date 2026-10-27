package kimi

import (
	"context"
	"encoding/json"
	"strings"
)

type cronTracker struct {
	jobs  map[string]bool
	dirty bool
}

func newCronTracker() cronTracker { return cronTracker{jobs: map[string]bool{}} }
func (c *cronTracker) restore(jobs map[string]bool) {
	if jobs != nil {
		c.jobs = jobs
	}
}
func (c *cronTracker) active() bool { return len(c.jobs) > 0 }
func (c *cronTracker) fired(raw json.RawMessage) {
	var fired struct {
		Origin struct {
			JobID string `json:"jobId"`
			Stale bool   `json:"stale"`
		} `json:"origin"`
	}
	if json.Unmarshal(raw, &fired) == nil {
		if recurring, ok := c.jobs[fired.Origin.JobID]; ok && (!recurring || fired.Origin.Stale) {
			delete(c.jobs, fired.Origin.JobID)
			c.dirty = true
		}
	}
}

// Native cron jobs keep the session running between model turns. Their
// lifecycle is recorded in tool results and cron.fired notifications. Mirror
// IDs (never prompts) in session metadata so Remote can retain that behavior
// when resuming its own sessions.
func (c *cronTracker) toolResult(t *childTool) {
	if t.IsError {
		return
	}
	switch t.Name {
	case "CronCreate":
		for _, job := range parseCronJobs(t.Output) {
			c.jobs[job.id] = job.recurring
			c.dirty = true
		}
	case "CronDelete":
		var args struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(t.Input, &args) == nil && args.ID != "" {
			delete(c.jobs, args.ID)
			c.dirty = true
		}
	case "CronList":
		if strings.HasPrefix(t.Output, "cron_jobs:") {
			c.jobs = map[string]bool{}
			for _, job := range parseCronJobs(t.Output) {
				c.jobs[job.id] = job.recurring
			}
			c.dirty = true
		}
	}
}

type cronJob struct {
	id        string
	recurring bool
}

func parseCronJobs(output string) []cronJob {
	var jobs []cronJob
	for _, line := range strings.Split(output, "\n") {
		if id, ok := strings.CutPrefix(line, "id: "); ok && id != "" {
			jobs = append(jobs, cronJob{id: id, recurring: true})
		}
		if len(jobs) > 0 && line == "recurring: false" {
			jobs[len(jobs)-1].recurring = false
		}
	}
	return jobs
}
func (c *cronTracker) save(ctx context.Context, p *serverTransport, sessionPath string) error {
	if !c.dirty {
		return nil
	}
	if err := p.api(ctx, "POST", sessionPath+"/profile", nativeProfileUpdate{Metadata: &nativeCronMetadata{Jobs: c.jobs}}, nil); err != nil {
		return err
	}
	c.dirty = false
	return nil
}

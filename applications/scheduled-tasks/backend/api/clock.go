package api

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"time"
)

func (a *API) loop() {
	for range time.Tick(time.Second) {
		if err := a.tick(); err != nil {
			fmt.Fprintf(os.Stderr, "scheduled tasks: %v\n", err)
		}
	}
}
func (a *API) tick() error {
	due, err := a.claimDue()
	if err != nil {
		return err
	}
	var result error
	for _, t := range due {
		// Events remain observability; delivery uses an acknowledged SDK call.
		if a.events != nil {
			_ = a.events.Due(t.ID, t.ActiveRunID)
		}
		if err := a.execute(t); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

// claimDue durably claims every due task before its event is published.
func (a *API) claimDue() ([]Task, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	updated := maps.Clone(a.tasks)
	due := []Task{}
	skipped := false
	for id, t := range updated {
		if !t.Enabled && t.ActiveRunID == "" {
			continue
		}
		if t.ActiveRunID == "" {
			if t.NextRunAt <= 0 || t.NextRunAt > now.UnixMilli() {
				continue
			}
			if now.UnixMilli()-t.NextRunAt > missedRunGrace.Milliseconds() {
				// The run was missed while Remote was stopped or updating. Running it
				// now would deliver a stale prompt, so skip it instead.
				updated[id] = a.skipMissed(t, now)
				skipped = true
				continue
			}
			runID, err := newID()
			if err != nil {
				return nil, err
			}
			t.ActiveRunID = runID
			updated[id] = t
		}
		if !a.retry[id].After(now) {
			due = append(due, t)
		}
	}
	if len(due) > 0 || skipped {
		if err := a.commit(updated); err != nil {
			return nil, err
		}
		for _, t := range due {
			a.retry[t.ID] = now.Add(retryInterval)
		}
	}
	return due, nil
}

// skipMissed drops a run that is long overdue. A one-time task is archived; a
// recurring task moves to its next future occurrence.
func (a *API) skipMissed(t Task, now time.Time) Task {
	t.LastError = "skipped: missed its scheduled time while Remote was not running"
	if t.Kind == "cron" {
		if next, err := nextOccurrence(t, now); err == nil {
			t.NextRunAt = next.UnixMilli()
			return t
		}
	}
	t.Enabled = false
	t.Archived = true
	t.NextRunAt = 0
	return t
}

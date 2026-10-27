package api

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

func (a *API) execute(task Task) error {
	if a.turns == nil {
		return nil
	}
	context, _ := json.Marshal(map[string]string{"taskId": task.ID, "runId": task.ActiveRunID})
	turn, err := a.turns.Start(applications.AgentTurnRequest{
		RequestID: task.ActiveRunID, ChatID: task.ChatID, OwnerEmail: task.OwnerEmail,
		Prompt: fmt.Sprintf("[Scheduled task: %s]\n\n%s", task.Name, task.Prompt), Context: context,
	})
	if err != nil {
		return a.executionError(task, err)
	}
	if turn.Status == "running" {
		a.mu.Lock()
		if current, ok := a.tasks[task.ID]; ok && current.ActiveRunID == task.ActiveRunID {
			a.retry[task.ID] = a.now().Add(time.Second)
		}
		a.mu.Unlock()
	}
	if turn.Status == "succeeded" || turn.Status == "failed" {
		if err := a.finish(task.ID, task.ActiveRunID, turn); err != nil {
			return err
		}
		// Forget only after the application's durable acknowledgment. Losing the
		// process before this point replays the receipt, not the agent work.
		return a.turns.Forget(task.ActiveRunID)
	}
	return nil
}
func (a *API) executionError(task Task, err error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	current, ok := a.tasks[task.ID]
	if !ok || current.ActiveRunID != task.ActiveRunID {
		return nil
	}
	current.LastError = err.Error()
	updated := maps.Clone(a.tasks)
	updated[task.ID] = current
	return a.commit(updated)
}
func (a *API) finish(id, runID string, turn applications.AgentTurn) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	task, ok := a.tasks[id]
	if !ok || task.ActiveRunID != runID {
		return nil
	}
	task.LastError = turn.Error
	task.ActiveRunID = ""
	task.RunCount++
	task.LastRunAt = a.now().UnixMilli()
	last := ""
	for _, line := range strings.Split(turn.Output, "\n") {
		if strings.TrimSpace(line) != "" {
			last = strings.TrimSpace(line)
		}
	}
	complete := last == "TASK_COMPLETE" || last == "SCHEDULE_STATUS=COMPLETE"
	if task.Kind == "once" || complete || (task.MaxRuns > 0 && task.RunCount >= task.MaxRuns) {
		task.Enabled = false
		task.Archived = true
		task.NextRunAt = 0
	} else if task.Enabled {
		next, err := nextOccurrence(task, a.now())
		if err != nil {
			task.Enabled = false
			task.LastError = err.Error()
		} else {
			task.NextRunAt = next.UnixMilli()
		}
	}
	updated := maps.Clone(a.tasks)
	updated[id] = task
	if err := a.commit(updated); err != nil {
		return err
	}
	delete(a.retry, id)
	return nil
}

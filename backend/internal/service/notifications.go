package service

import (
	"context"
	"fmt"
	"strings"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicenotify "github.com/futrx-com/remote.futrx.com/internal/service/notify"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	serviceschedule "github.com/futrx-com/remote.futrx.com/internal/service/schedule"
)

// notifyObserver translates run and schedule lifecycle signals into outbound
// notification events. It lives in the composition package because it is the
// only place allowed to reach across the chat, project, and notify services.
type notifyObserver struct {
	notifications *servicenotify.Service
	chats         servicechat.Repository
	projects      *serviceproject.Service
}

var (
	_ prompt.RunObserver          = (*notifyObserver)(nil)
	_ serviceschedule.RunObserver = (*notifyObserver)(nil)
)

// RunSettled reports a finished interactive run. Scheduled runs are skipped:
// the schedule service reports those with richer task context.
func (o *notifyObserver) RunSettled(ctx context.Context, outcome prompt.RunOutcome) {
	if o == nil || outcome.ScheduledTaskID != "" {
		return
	}
	kind, status := servicenotify.RunKind(outcome.Err, outcome.Cancelled)
	event := servicenotify.Event{
		Event:     kind,
		Status:    status,
		Summary:   runSummary(outcome),
		DedupeKey: fmt.Sprintf("run:%s:%d", outcome.ChatID, outcome.RunID),
	}
	o.describeChat(ctx, outcome.ChatID, &event)
	o.notifications.Publish(event)
}

// RunToolStarted reports the tool calls that hand control back to the human.
// Every such call is its own event: an agent that asks twice pings twice.
func (o *notifyObserver) RunToolStarted(ctx context.Context, chatID servicechat.ID, toolName string) {
	if o == nil || !servicenotify.NeedsAttentionTool(toolName) {
		return
	}
	event := servicenotify.Event{
		Event:   servicenotify.KindNeedsAttention,
		Status:  servicenotify.StatusWaiting,
		Summary: "The agent called " + toolName + " and is waiting for a human decision.",
	}
	o.describeChat(ctx, chatID, &event)
	o.notifications.Publish(event)
}

// ScheduledRunFinished reports the outcome of a scheduled task run.
func (o *notifyObserver) ScheduledRunFinished(
	ctx context.Context,
	task serviceschedule.Task,
	result serviceschedule.RunResult,
) {
	if o == nil {
		return
	}
	status := servicenotify.StatusSucceeded
	detail := result.Output
	if result.Err != nil {
		status = servicenotify.StatusFailed
		detail = result.Err.Error()
	}
	event := servicenotify.Event{
		Event:     servicenotify.KindScheduledRun,
		Status:    status,
		Summary:   scheduleSummary(task.Name, detail),
		DedupeKey: fmt.Sprintf("schedule:%s:%d", task.ID, task.LastRunAt),
	}
	o.describeChat(ctx, task.ChatID, &event)
	o.notifications.Publish(event)
}

// describeChat fills the chat and project identity fields of an event. Lookup
// failures leave the fields empty rather than dropping the notification.
func (o *notifyObserver) describeChat(
	ctx context.Context,
	chatID servicechat.ID,
	event *servicenotify.Event,
) {
	event.ChatID = string(chatID)
	if o.chats == nil || chatID == "" {
		return
	}
	// A cancelled run hands us a cancelled context; the lookups behind this
	// enrichment are cheap and local, so run them anyway.
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	meta, err := o.chats.Get(ctx, chatID)
	if err != nil {
		return
	}
	event.ChatTitle = meta.Title
	event.Provider = string(meta.Provider)
	if meta.ProjectID == "" || o.projects == nil {
		return
	}
	event.ProjectID = string(meta.ProjectID)
	project, err := o.projects.Get(ctx, serviceproject.ID(meta.ProjectID))
	if err != nil {
		return
	}
	event.ProjectName = project.Name
	event.ProjectSlug = project.Slug
}

// runSummary prefers the agent's own last words and falls back to the failure
// reason so a failure notification is never empty.
func runSummary(outcome prompt.RunOutcome) string {
	if outcome.Cancelled {
		return "The run was cancelled before it finished."
	}
	if outcome.Err != nil {
		return servicenotify.Summary(outcome.Err.Error())
	}
	return servicenotify.Summary(outcome.Output)
}

// scheduleSummary keeps the task name in the body, since the event payload has
// no dedicated task field.
func scheduleSummary(taskName, detail string) string {
	detail = servicenotify.Summary(detail)
	taskName = strings.TrimSpace(taskName)
	switch {
	case taskName == "":
		return detail
	case detail == "":
		return "Task: " + taskName
	default:
		return "Task: " + taskName + "\n" + detail
	}
}

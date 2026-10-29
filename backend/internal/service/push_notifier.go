package service

import (
	"context"
	"log"
	"strings"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicepresence "github.com/futrx-com/remote.futrx.com/internal/service/presence"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
)

// audienceTimeout bounds the store reads that resolve who to notify. The
// delivery itself runs separately, on the push service's own deadline.
const audienceTimeout = 5 * time.Second

// chatPushNotifier turns persisted chat events into push notifications. It
// hangs off the chat repository so every path that appends an event —
// interactive prompts, application turns, recovery — is covered by construction.
type chatPushNotifier struct {
	push     *servicepush.Service
	chats    servicechat.Repository
	projects interface {
		Get(context.Context, serviceproject.ID) (serviceproject.Meta, error)
	}
	audience chatNotificationAudience
	presence *servicepresence.Service

	state chatNotificationState
}

// ChatEvent decides whether an appended event deserves a notification and, if
// so, who receives it. It never blocks the caller on network I/O.
func (n *chatPushNotifier) ChatEvent(chatID servicechat.ID, event servicechat.Event) {
	if n == nil || !n.push.Enabled() {
		return
	}
	n.state.trackUserPrompt(chatID, event)
	kind, urgent, ok := notificationKind(event)
	if !n.state.trackParkedRun(chatID, event) {
		return
	}
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), audienceTimeout)
	defer cancel()

	meta, err := n.chats.Get(ctx, chatID)
	if err != nil {
		log.Printf("push: resolve chat %s: %v", chatID, err)
		return
	}
	recipients, err := n.audience.recipients(ctx, meta)
	if err != nil {
		log.Printf("push: resolve audience for chat %s: %v", chatID, err)
		return
	}
	// Someone with this chat on screen is already watching the thing the
	// notification would announce. Dropping them here silences every device
	// they own, which the service worker cannot do: it only sees the tabs of
	// the browser it runs in, so their other phone would buzz regardless.
	recipients = n.presence.Filter(recipients, string(chatID))
	if len(recipients) == 0 {
		return
	}
	batch := questionBatchFromEvent(event)
	claim, ok := n.state.claimUnreadSlot(chatID, meta.LastReadAt, kind == servicepush.KindQuestion, batch)
	if !ok {
		return
	}

	projectName := "Remote"
	if meta.ProjectID != "" && n.projects != nil {
		if project, err := n.projects.Get(ctx, serviceproject.ID(meta.ProjectID)); err == nil {
			projectName = project.Name
		}
	}
	title, body := notificationText(kind, projectName, event)
	n.push.NotifyAsync(recipients, servicepush.Notification{
		Kind:   kind,
		ChatID: string(chatID),
		Title:  title,
		Body:   body,
		// One tag per chat: a later notification replaces the chat's earlier
		// tray entry instead of stacking behind it.
		Tag:    "chat:" + string(chatID),
		Urgent: urgent,
	}, func(delivered int) {
		if delivered == 0 {
			n.state.releaseUnreadSlot(chatID, claim, batch)
		}
	})
}

// ChatDeleted drops everything held for a chat that no longer exists.
func (n *chatPushNotifier) ChatDeleted(chatID servicechat.ID) {
	if n == nil {
		return
	}
	n.state.deleteChat(chatID)
}

// notificationKind maps a chat event onto a notification, or reports that the
// event is not worth interrupting anyone for. Streaming deltas, tool traffic,
// and session bookkeeping all fall through.
func notificationKind(event servicechat.Event) (kind servicepush.Kind, urgent, ok bool) {
	if servicechat.IsQuestionRequest(event) {
		return servicepush.KindQuestion, true, true
	}
	application := event.ApplicationID != ""
	legacyScheduled := strings.TrimSpace(event.ScheduledTaskID) != ""
	switch event.Type {
	case "complete":
		if application {
			return servicepush.KindApplication, false, true
		}
		if legacyScheduled {
			return servicepush.KindScheduled, false, true
		}
		return servicepush.KindComplete, false, true
	case "error":
		if application {
			return servicepush.KindApplication, false, true
		}
		if legacyScheduled {
			return servicepush.KindScheduled, false, true
		}
		return servicepush.KindError, false, true
	default:
		return "", false, false
	}
}

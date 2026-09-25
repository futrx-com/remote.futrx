package service

import (
	"strings"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
)

// notificationText owns the title and bounded body sent through browser push.
func notificationText(
	kind servicepush.Kind,
	projectName string,
	event servicechat.Event,
) (title, body string) {
	projectName = truncatePushText(strings.TrimSpace(projectName), 120)
	if projectName == "" {
		projectName = "Remote"
	}

	switch kind {
	case servicepush.KindQuestion:
		return projectName + " - Agent needs your answer", "Open the chat to answer the question."
	case servicepush.KindComplete:
		return projectName + " - Agent finished", completionBody(event, "Open the chat to see the result.")
	case servicepush.KindError:
		return projectName + " - Agent encountered an error", errorBody(event.Message)
	case servicepush.KindScheduled:
		if event.Type == "error" {
			return projectName + " - Agent encountered an error", errorBody(event.Message)
		}
		return projectName + " - Agent finished", completionBody(event, "A scheduled task finished.")
	default:
		return projectName, ""
	}
}

func errorBody(detail string) string {
	detail = strings.Join(strings.Fields(detail), " ")
	if detail == "" {
		return "Open the chat to review the error."
	}
	return truncatePushText(detail, 180)
}

func completionBody(event servicechat.Event, fallback string) string {
	summary := strings.Join(strings.Fields(event.NotificationSummary), " ")
	if summary == "" {
		return fallback
	}
	return truncatePushText(summary, 240)
}

func truncatePushText(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	cut := 0
	for index := range value {
		if index > maxBytes-3 {
			break
		}
		cut = index
	}
	return value[:cut] + "…"
}

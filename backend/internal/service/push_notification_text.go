package service

import (
	"encoding/json"
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
	finishedTitle := projectName + " - Agent finished ✅"
	errorTitle := projectName + " - Agent encountered an error ❌"

	switch kind {
	case servicepush.KindQuestion:
		return projectName + " - Agent needs your answer ❓", questionBody(event)
	case servicepush.KindComplete:
		return finishedTitle, completionBody(event, "Open the chat to see the result.")
	case servicepush.KindError:
		return errorTitle, errorBody(event.Message)
	case servicepush.KindApplication:
		if event.Type == "error" {
			return errorTitle, errorBody(event.Message)
		}
		return finishedTitle, completionBody(event, "An application-started turn finished.")
	case servicepush.KindScheduled:
		if event.Type == "error" {
			return errorTitle, errorBody(event.Message)
		}
		return finishedTitle, completionBody(event, "A scheduled task finished.")
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
	return boundedSummary(summary)
}

func questionBody(event servicechat.Event) string {
	const fallback = "Open the chat to answer the question."
	var input struct {
		Questions []struct {
			Question string `json:"question"`
			IsSecret bool   `json:"isSecret"`
		} `json:"questions"`
	}
	if json.Unmarshal(event.Input, &input) != nil {
		return fallback
	}
	var questions []string
	for _, question := range input.Questions {
		text := strings.Join(strings.Fields(question.Question), " ")
		if text != "" && !question.IsSecret {
			questions = append(questions, text)
		}
	}
	if len(questions) == 0 {
		return fallback
	}
	return boundedSummary(strings.Join(questions, " · "))
}

func boundedSummary(summary string) string {
	runes := []rune(summary)
	if len(runes) > 200 {
		summary = string(runes[:199]) + "…"
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

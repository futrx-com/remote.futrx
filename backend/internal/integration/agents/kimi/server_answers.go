package kimi

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type nativeInteractionReply struct {
	Path string
	Body any
}
type nativeApprovalAnswer struct {
	Decision      string `json:"decision"`
	Scope         string `json:"scope,omitempty"`
	Feedback      string `json:"feedback,omitempty"`
	SelectedLabel string `json:"selected_label,omitempty"`
}
type nativeQuestionAnswers struct {
	Answers map[string]nativeQuestionAnswer `json:"answers"`
	Method  string                          `json:"method"`
}
type nativeQuestionAnswer struct {
	Kind      string   `json:"kind"`
	OptionID  string   `json:"option_id,omitempty"`
	OptionIDs []string `json:"option_ids,omitempty"`
	Text      string   `json:"text,omitempty"`
	OtherText string   `json:"other_text,omitempty"`
}

type interactionResult = struct {
	Decision      string `json:"decision"`
	Feedback      string `json:"feedback"`
	SelectedLabel string `json:"selected_label"`
	Dismiss       bool   `json:"dismiss"`
	Answers       map[string]struct {
		Answers []string `json:"answers"`
	} `json:"answers"`
}

// reply validates an answer against the original request without doing I/O or
// consuming pending identity. Transport success determines resolution.
func (pending pendingInteraction) reply(response agent.InteractionResponse) (nativeInteractionReply, error) {
	var result interactionResult
	if len(response.Result) > 0 {
		if err := json.Unmarshal(response.Result, &result); err != nil {
			return nativeInteractionReply{}, fmt.Errorf("invalid Kimi interaction response: %w", err)
		}
	}
	path := "/" + pending.kind + "s/" + url.PathEscape(pending.id)
	var body any = struct{}{}
	if pending.kind == "approval" {
		approval, err := approvalAnswer(result, len(response.Error) > 0)
		if err != nil {
			return nativeInteractionReply{}, err
		}
		body = approval
	} else if result.Dismiss || len(response.Error) > 0 {
		path += ":dismiss"
	} else {
		answers := map[string]nativeQuestionAnswer{}
		for _, q := range pending.questions {
			values, exists := result.Answers[q.ID]
			if !exists {
				return nativeInteractionReply{}, fmt.Errorf("missing answer to Kimi question %s", q.ID)
			}
			answer, err := q.answer(values.Answers)
			if err != nil {
				return nativeInteractionReply{}, err
			}
			answers[q.ID] = answer
		}
		body = nativeQuestionAnswers{Answers: answers, Method: "click"}
	}
	return nativeInteractionReply{Path: path, Body: body}, nil
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func approvalAnswer(result interactionResult, hasError bool) (nativeApprovalAnswer, error) {
	approval := nativeApprovalAnswer{}
	decision := ""
	switch result.Decision {
	case "accept", "approved":
		decision = "approved"
	case "acceptForSession":
		decision = "approved"
		approval.Scope = "session"
	case "decline", "rejected":
		decision = "rejected"
	case "cancel", "cancelled":
		decision = "cancelled"
	}
	if hasError {
		decision = "rejected"
	}
	if decision == "" {
		return nativeApprovalAnswer{}, fmt.Errorf("invalid Kimi approval decision")
	}
	approval.Decision = decision
	if result.Feedback != "" {
		approval.Feedback = result.Feedback
	}
	if result.SelectedLabel != "" {
		approval.SelectedLabel = result.SelectedLabel
	}
	return approval, nil
}

func (q nativeQuestion) answer(values []string) (nativeQuestionAnswer, error) {
	ids := []string{}
	other := ""
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		found := ""
		for _, option := range q.Options {
			if value == option.ID {
				found = option.ID
				break
			}
		}
		if found == "" {
			if !q.AllowOther || other != "" {
				return nativeQuestionAnswer{}, fmt.Errorf("invalid answer to Kimi question %s", q.ID)
			}
			other = value
		} else {
			ids = append(ids, found)
		}
	}
	if !q.MultiSelect && len(ids)+boolInt(other != "") > 1 {
		return nativeQuestionAnswer{}, fmt.Errorf("Kimi question %s allows one answer", q.ID)
	}
	answer := nativeQuestionAnswer{Kind: "skipped"}
	switch {
	case other != "" && len(ids) > 0:
		answer = nativeQuestionAnswer{Kind: "multi_with_other", OptionIDs: ids, OtherText: other}
	case other != "":
		answer = nativeQuestionAnswer{Kind: "other", Text: other}
	case q.MultiSelect && len(ids) > 0:
		answer = nativeQuestionAnswer{Kind: "multi", OptionIDs: ids}
	case len(ids) == 1:
		answer = nativeQuestionAnswer{Kind: "single", OptionID: ids[0]}
	}
	return answer, nil
}
